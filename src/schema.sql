-- 盘州市人民医院网络安全应急演练系统 · SQLite 表结构
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- 角色（nav 为 JSON 数组）
CREATE TABLE IF NOT EXISTS roles (
  id    TEXT PRIMARY KEY,
  name  TEXT NOT NULL,
  short TEXT,
  icon  TEXT,
  color TEXT,
  descr TEXT,
  nav   TEXT,
  external INTEGER DEFAULT 0,
  seq   INTEGER DEFAULT 0
);

-- 用户账号
CREATE TABLE IF NOT EXISTS users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  username   TEXT UNIQUE NOT NULL,
  name       TEXT NOT NULL,
  role_id    TEXT NOT NULL,
  org        TEXT,
  pwd_salt   TEXT NOT NULL,
  pwd_hash   TEXT NOT NULL,
  status     TEXT DEFAULT '启用',
  must_change INTEGER DEFAULT 1,
  created_at TEXT
);

-- 登录会话
CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL,
  created_at TEXT,
  expires_at TEXT
);

-- 演练项目
CREATE TABLE IF NOT EXISTS projects (
  id          TEXT PRIMARY KEY,
  name        TEXT,
  type        TEXT,
  date        TEXT,
  scenario_id TEXT,
  level       TEXT,
  status      TEXT,
  target      TEXT,
  units       TEXT,   -- JSON
  members     TEXT,   -- JSON
  observers   TEXT,   -- JSON
  mode        TEXT    -- 演练模式：基础版（诱导演练）/ 实战版（自主处置）
);

-- 演练场景剧本
CREATE TABLE IF NOT EXISTS scenarios (
  id         TEXT PRIMARY KEY,
  name       TEXT,
  type       TEXT,
  level      TEXT,
  difficulty TEXT,
  status     TEXT,
  version    TEXT,
  updated    TEXT,
  nodes      INTEGER,
  descr      TEXT
);

-- 剧本节点
CREATE TABLE IF NOT EXISTS script_nodes (
  id         TEXT PRIMARY KEY,
  scenario_id TEXT,
  type       TEXT,
  title      TEXT,
  target     TEXT,
  time       TEXT,
  timeout    INTEGER,
  form       TEXT,
  att        TEXT,
  descr      TEXT,
  branches   TEXT,   -- JSON
  seq        INTEGER,
  guide      TEXT,   -- 基础版引导提示（实战版不下发）
  keywords   TEXT,   -- 实战版判定关键词（逗号分隔）
  action     TEXT,   -- 对应动作类型（实战版步骤归类）
  required   INTEGER DEFAULT 1  -- 是否基础版必经步骤（实战版据此判定遗漏）
);

-- 实战版自主处置步骤（参演人员自行添加，系统按基础版判定遗漏与时限）
CREATE TABLE IF NOT EXISTS drill_steps (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id  TEXT,
  scenario_id TEXT,
  role_id     TEXT,
  user        TEXT,
  action      TEXT,   -- 动作类型
  title       TEXT,
  descr       TEXT,
  step_time   TEXT,   -- 实际处置时刻 HH:mm
  matched_node TEXT,  -- 匹配到的剧本节点ID（空=未匹配/冗余）
  matched     INTEGER DEFAULT 0,
  ontime      INTEGER DEFAULT 0,
  created_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_drill_steps_proj ON drill_steps(project_id);

-- 演练任务
CREATE TABLE IF NOT EXISTS tasks (
  id         TEXT PRIMARY KEY,
  project_id TEXT,
  role_id    TEXT,
  title      TEXT,
  descr      TEXT,
  deadline   TEXT,
  status     TEXT,
  level      TEXT,
  forms      TEXT,   -- JSON
  seq        INTEGER
);

-- 任务附件（真实文件）
CREATE TABLE IF NOT EXISTS task_attachments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id    TEXT NOT NULL,
  name       TEXT NOT NULL,
  type       TEXT,
  size       TEXT,
  url        TEXT,          -- data:image 缩略图或 /uploads/xxx
  file_path  TEXT,          -- 相对 uploads 的真实文件路径
  uploader   TEXT,
  uploader_id INTEGER,
  time       TEXT,
  auto       INTEGER DEFAULT 0
);

-- 表单模板
CREATE TABLE IF NOT EXISTS form_templates (
  id     TEXT PRIMARY KEY,
  name   TEXT,
  fields TEXT   -- JSON
);

-- 表单提交记录
CREATE TABLE IF NOT EXISTS form_submissions (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id   TEXT,
  form_id   TEXT,
  form_name TEXT,
  data      TEXT,   -- JSON
  submitter TEXT,
  user_id   INTEGER,
  role_id   TEXT,
  time      TEXT
);

-- 消息流
CREATE TABLE IF NOT EXISTS messages (
  id      TEXT PRIMARY KEY,
  time    TEXT,
  level   TEXT,
  source  TEXT,
  text    TEXT,
  targets TEXT,   -- JSON
  seq     INTEGER
);

-- 通知
CREATE TABLE IF NOT EXISTS notifs (
  id    TEXT PRIMARY KEY,
  time  TEXT,
  level TEXT,
  source TEXT,
  title TEXT,
  descr TEXT,
  seq   INTEGER
);

-- 未读状态（按用户）
CREATE TABLE IF NOT EXISTS notif_reads (
  user_id INTEGER,
  notif_id TEXT,
  PRIMARY KEY (user_id, notif_id)
);

-- 会商消息
CREATE TABLE IF NOT EXISTS chat_messages (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  grp     TEXT NOT NULL,
  sender  TEXT,
  role_id TEXT,
  user_id INTEGER,
  time    TEXT,
  text    TEXT,
  att     TEXT,
  file_path TEXT
);

-- 取证材料
CREATE TABLE IF NOT EXISTS evidence (
  id         TEXT PRIMARY KEY,
  type       TEXT,
  name       TEXT,
  size       TEXT,
  time       TEXT,
  role_id    TEXT,
  uploader   TEXT,
  user_id    INTEGER,
  auto       INTEGER DEFAULT 0,
  task_id    TEXT,
  project_id TEXT,
  url        TEXT,
  file_path  TEXT,
  created_at TEXT
);

-- 状态快照
CREATE TABLE IF NOT EXISTS snapshots (
  id         TEXT PRIMARY KEY,
  project_id TEXT,
  name       TEXT,
  time       TEXT,
  by_user    TEXT,
  note       TEXT
);

-- 时序记录（演练台账）
CREATE TABLE IF NOT EXISTS timeline (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT,
  t          TEXT,
  actor      TEXT,
  role_id    TEXT,
  action     TEXT,
  detail     TEXT,
  level      TEXT,
  seq        INTEGER
);

-- 处置时序图节点
CREATE TABLE IF NOT EXISTS seq_flow (
  id    INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT,
  name  TEXT,
  time  TEXT,
  role  TEXT,
  state TEXT,
  seq   INTEGER
);

-- 评分指标
CREATE TABLE IF NOT EXISTS score_items (
  id     TEXT PRIMARY KEY,
  name   TEXT,
  weight INTEGER,
  rule   TEXT,
  seq    INTEGER
);

-- 评分结果
CREATE TABLE IF NOT EXISTS scores (
  item_id    TEXT PRIMARY KEY,
  project_id TEXT,
  score      REAL,
  max        INTEGER,
  note       TEXT,
  auto       INTEGER DEFAULT 1,
  updated_at TEXT
);

-- 演练台账
CREATE TABLE IF NOT EXISTS ledger (
  id         TEXT PRIMARY KEY,
  project_id TEXT,
  name       TEXT,
  date       TEXT,
  type       TEXT,
  event      TEXT,
  level      TEXT,
  result     TEXT,
  score      TEXT,
  org        TEXT
);

-- 舆情记录
CREATE TABLE IF NOT EXISTS opinion (
  id      TEXT PRIMARY KEY,
  project_id TEXT,
  time    TEXT,
  channel TEXT,
  title   TEXT,
  heat    TEXT,
  risk    TEXT,
  status  TEXT,
  source  TEXT
);

-- 预案知识库
CREATE TABLE IF NOT EXISTS knowledge (
  id      TEXT PRIMARY KEY,
  name    TEXT,
  cat     TEXT,
  updated TEXT,
  size    TEXT,
  pages   INTEGER,
  content TEXT,
  file_path TEXT
);

-- 审计日志（仅追加，不提供删除接口）
CREATE TABLE IF NOT EXISTS audit_logs (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  time    TEXT,
  user    TEXT,
  role_id TEXT,
  user_id INTEGER,
  action  TEXT,
  ip      TEXT
);

-- 业务影响
CREATE TABLE IF NOT EXISTS impact (
  sys   TEXT PRIMARY KEY,
  state TEXT,
  pct   INTEGER,
  seq   INTEGER
);

-- 数据字典
CREATE TABLE IF NOT EXISTS dict (
  key   TEXT PRIMARY KEY,
  value TEXT   -- JSON
);

-- 场景模拟数据（自动填充/自动取证）
CREATE TABLE IF NOT EXISTS sim_data (
  type TEXT PRIMARY KEY,
  data TEXT   -- JSON
);

-- 系统设置
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT
);

CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_role ON tasks(role_id);
CREATE INDEX IF NOT EXISTS idx_att_task ON task_attachments(task_id);
CREATE INDEX IF NOT EXISTS idx_evidence_project ON evidence(project_id);
CREATE INDEX IF NOT EXISTS idx_timeline_project ON timeline(project_id);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(time);
CREATE INDEX IF NOT EXISTS idx_chat_grp ON chat_messages(grp);
