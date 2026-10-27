/* 前端运行时缓存容器 window.MOCK
 * 种子数据源已迁移至 src/seed_data.js
 * 此文件仅初始化空对象；真实数据由 DRILL.bootstrap() → /api/bootstrap 从数据库拉取后填充
 * api.js / admin.html 等页面代码在 bootstrap 后从 window.MOCK 读取，无需关心真实数据源位置
 */
window.MOCK = window.MOCK || {};
