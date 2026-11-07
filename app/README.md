# 元小智三环节系统（app）

按 `../项目开发/` 的开发文档实现。已完成：**计算 `/jisuan`**、**输出 `/shuchu`**；输入 `/shuru` 只有占位页。

## 启动

```bash
cp .env.example .env      # 填模型网关、密钥、教师口令
make run                  # 构建前端 + Go 单文件程序，并启动 http://localhost:8080
```

或者用 Docker：`docker compose up -d`。不能装 Docker 时，`make dist` 生成 Windows / macOS 单文件程序。

| 页面 | 地址 |
|---|---|
| 入口（二维码） | `/` |
| 计算 · 平板 / 大屏 | `/jisuan/`、`/jisuan/screen` |
| 输出 · 平板 / 大屏 | `/shuchu/`、`/shuchu/screen` |
| 教师控制台 | `/admin` |

大屏快捷键：`Space` 操作条、`←/→` 切换视图、数字键选平板（输出环节选组）、`P` 继续（规划器暂停时）、`R` 重播、`F` 全屏、`Esc` 回总览。

## 目录

- `server/`：Go（只用标准库）。`internal/core` 路由、SSE、座位、口令、快照；`internal/llm` 模型网关（HTTP/1.1 流式、排队、自动降并发、回放录制）；`internal/jisuan` 出题、画像、打分、候选、模型推理与校验；`internal/shuchu` 道路网、规划器、核验、画图说明书。
- `web/`：Vue 3 + Vite 多页面，构建产物嵌进 Go 程序。
- `content/`：书库、标签、提示词、道路网、班级、信息卡（改内容不用改代码）。
- `data/`：运行时状态快照、日志、回放录音、学生收到的路线图 JPEG（`./bin/yxz purge` 清除）。

## 自检

- `./bin/yxz check`：校验内容文件（出题不重复、每个班至少 2 部楼梯、信息齐全时规划器一定出现红色楼梯）。
- 2026-11-07 实测：真实模型分析 11–25 秒；25 台平板并发（模拟模式）每轮换书平均 16 ms，零失败；路线图 JPEG 约 160 KB。
