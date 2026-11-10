package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// -----------------------------------------------------------------------------
// 备份加密（AES-256-GCM，口令派生密钥）
//
// 文件格式（DRLBK1）：
//   magic  7B  "DRLBK1\x00"
//   salt   16B 随机盐
//   iter    4B PBKDF2-SHA256 迭代次数（大端）
//   帧序列：[nonce 12B][ctLen 4B][ciphertext+tag]
//
// 每帧 AAD = header || 帧序号，防止跨文件/跨帧拼接。口令只从环境变量或
// 密钥文件读取，不写日志、不写备份本身；备份文件与目录分别强制 600 / 700。
// -----------------------------------------------------------------------------

const (
	backupMagic      = "DRLBK1\x00"
	backupSaltLen    = 16
	backupNonceLen   = 12
	backupKeyLen     = 32
	backupPBKDF2Iter = 600000
	backupChunkSize  = 1 << 20 // 1 MiB
	// restoreConfirmWord 恢复接口的二次确认字面量
	restoreConfirmWord = "RESTORE"
)

// errBackupPassphraseWrong 解不开时的统一错误（不区分口令错与文件损坏，避免探测）
var errBackupPassphraseWrong = errors.New("备份口令不正确，或备份文件已被篡改")

// ensurePrivateDir 创建目录并强制 0700。
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// backupPassphrase 读取备份加密口令：优先 BACKUP_PASSPHRASE，其次 BACKUP_PASSPHRASE_FILE。
func backupPassphrase() (string, error) {
	if p := strings.TrimSpace(os.Getenv("BACKUP_PASSPHRASE")); p != "" {
		return p, nil
	}
	if f := strings.TrimSpace(os.Getenv("BACKUP_PASSPHRASE_FILE")); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("读取 BACKUP_PASSPHRASE_FILE 失败: %w", err)
		}
		if p := strings.TrimSpace(string(b)); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("BACKUP_PASSPHRASE_FILE 内容为空: %s", f)
	}
	return "", errors.New("未配置备份加密口令（需设置 BACKUP_PASSPHRASE 或 BACKUP_PASSPHRASE_FILE）")
}

// checkRestoreGate 校验恢复入口的二次确认与独立恢复口令。
// 返回 (0, "") 表示通过；否则返回 HTTP 状态码与错误文案。
func checkRestoreGate(confirm, password string) (int, string) {
	if strings.ToUpper(strings.TrimSpace(confirm)) != restoreConfirmWord {
		return http.StatusBadRequest, "恢复操作需要二次确认：请提交 confirm=" + restoreConfirmWord
	}
	expected := strings.TrimSpace(os.Getenv("RESTORE_PASSWORD"))
	if expected == "" {
		return http.StatusServiceUnavailable, "未配置独立恢复口令（RESTORE_PASSWORD），为安全起见已禁止恢复"
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(password)), []byte(expected)) != 1 {
		return http.StatusForbidden, "恢复口令错误"
	}
	return 0, ""
}

// isEncryptedBackup 判断文件是否为本程序产出的加密备份。
func isEncryptedBackup(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(backupMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return string(head) == backupMagic
}

// encryptBackupFile 把明文备份加密写入 dst；成功时 dst 权限为 600。
func encryptBackupFile(src, dst, passphrase string) error {
	if strings.TrimSpace(passphrase) == "" {
		return errors.New("备份加密口令为空")
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开待加密文件失败: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建加密备份失败: %w", err)
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()

	header := make([]byte, 0, len(backupMagic)+backupSaltLen+4)
	header = append(header, []byte(backupMagic)...)
	salt := make([]byte, backupSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("生成随机盐失败: %w", err)
	}
	header = append(header, salt...)
	iter := make([]byte, 4)
	binary.BigEndian.PutUint32(iter, backupPBKDF2Iter)
	header = append(header, iter...)

	if _, err := out.Write(header); err != nil {
		return fmt.Errorf("写入备份头失败: %w", err)
	}

	key := pbkdf2.Key([]byte(passphrase), salt, backupPBKDF2Iter, backupKeyLen, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("初始化加密器失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("初始化 GCM 失败: %w", err)
	}

	buf := make([]byte, backupChunkSize)
	var frame uint32
	for {
		n, readErr := io.ReadFull(in, buf)
		if n == 0 && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return fmt.Errorf("读取待加密文件失败: %w", readErr)
		}
		nonce := make([]byte, backupNonceLen)
		if _, err := rand.Read(nonce); err != nil {
			return fmt.Errorf("生成随机数失败: %w", err)
		}
		aad := frameAAD(header, frame)
		ct := gcm.Seal(nil, nonce, buf[:n], aad)
		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(ct)))
		if _, err := out.Write(nonce); err != nil {
			return fmt.Errorf("写入加密备份失败: %w", err)
		}
		if _, err := out.Write(lenBuf); err != nil {
			return fmt.Errorf("写入加密备份失败: %w", err)
		}
		if _, err := out.Write(ct); err != nil {
			return fmt.Errorf("写入加密备份失败: %w", err)
		}
		frame++
		if readErr == io.ErrUnexpectedEOF || readErr == io.EOF {
			break
		}
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭加密备份失败: %w", err)
	}
	if err := os.Chmod(dst, 0o600); err != nil {
		return fmt.Errorf("设置备份文件权限失败: %w", err)
	}
	ok = true
	return nil
}

// decryptBackupFile 解密加密备份到 dst；口令错误或文件损坏统一返回 errBackupPassphraseWrong。
func decryptBackupFile(src, dst, passphrase string) error {
	if strings.TrimSpace(passphrase) == "" {
		return errBackupPassphraseWrong
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开加密备份失败: %w", err)
	}
	defer in.Close()

	headLen := len(backupMagic) + backupSaltLen + 4
	header := make([]byte, headLen)
	if _, err := io.ReadFull(in, header); err != nil {
		return errBackupPassphraseWrong
	}
	if string(header[:len(backupMagic)]) != backupMagic {
		return errors.New("不是本程序生成的加密备份（文件头不符）")
	}
	salt := header[len(backupMagic) : len(backupMagic)+backupSaltLen]
	iter := binary.BigEndian.Uint32(header[headLen-4:])
	if iter == 0 || iter > 10_000_000 {
		return errors.New("加密备份头部的 KDF 参数非法")
	}

	key := pbkdf2.Key([]byte(passphrase), salt, int(iter), backupKeyLen, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("初始化解密器失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("初始化 GCM 失败: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建解密临时文件失败: %w", err)
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()

	nonce := make([]byte, backupNonceLen)
	lenBuf := make([]byte, 4)
	var frame uint32
	for {
		_, err := io.ReadFull(in, nonce)
		if err == io.EOF {
			break
		}
		if err != nil {
			return errBackupPassphraseWrong
		}
		if _, err := io.ReadFull(in, lenBuf); err != nil {
			return errBackupPassphraseWrong
		}
		ctLen := binary.BigEndian.Uint32(lenBuf)
		if ctLen == 0 || ctLen > uint32(backupChunkSize+gcm.Overhead()) {
			return errBackupPassphraseWrong
		}
		ct := make([]byte, ctLen)
		if _, err := io.ReadFull(in, ct); err != nil {
			return errBackupPassphraseWrong
		}
		plain, err := gcm.Open(nil, nonce, ct, frameAAD(header, frame))
		if err != nil {
			return errBackupPassphraseWrong
		}
		if _, err := out.Write(plain); err != nil {
			return fmt.Errorf("写入解密文件失败: %w", err)
		}
		frame++
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭解密文件失败: %w", err)
	}
	if err := os.Chmod(dst, 0o600); err != nil {
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	ok = true
	return nil
}

func frameAAD(header []byte, frame uint32) []byte {
	aad := make([]byte, len(header)+4)
	copy(aad, header)
	binary.BigEndian.PutUint32(aad[len(header):], frame)
	return aad
}

// encryptedBackupName 由明文名生成加密备份名（在原名后追加 .enc）。
func encryptedBackupName(name string) string {
	return name + ".enc"
}

// fileExists 判断路径是否为已存在的普通文件。
func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// baseNameOfEncrypted 去掉 .enc 后缀，用于定位同名附件归档 / 清单。
func baseNameOfEncrypted(name string) string {
	return strings.TrimSuffix(name, ".enc")
}
