# 元小智三环节系统（app）

按 `../项目开发/` 的开发文档实现。已完成：**计算 `/jisuan`**、**输出 `/shuchu`**、**输入 `/shuru`**（秋假攻略，见 `../项目开发/输入/01_开发文档_秋假攻略.md`）、**畅想 `/changxiang`**（学生设计元小智的新本领，大屏卷轴展示）。学生页顶栏可以在四个环节之间切换。换下一个班前，在 `/admin` 点“清空本节课数据”。

## 启动

```bash
cp .env.example .env      # 填模型网关、密钥、教师口令
make run                  # 构建前端 + Go 单文件程序，并启动 http://localhost:8080
```

或者用 Docker（镜像由 GitHub 自动构建，支持普通电脑和苹果芯片）：

```bash
docker pull ghcr.io/alumos/yuanxiaozhi_next:latest
docker compose up -d      # 需要当前目录下有 docker-compose.yml 和填好的 .env
```

在 1Panel 上部署：直接用 [deploy/1panel/docker-compose.yml](deploy/1panel/docker-compose.yml)（端口 38080，粘进 1Panel 的“编排”即可）。如果前面套了 1Panel 的反向代理网站，记得在代理配置里关掉缓冲（`proxy_buffering off;`），否则实时推送会卡住。

不能装 Docker 时，`make dist` 生成 Windows / macOS 单文件程序。

| 页面 | 地址 |
|---|---|
| 入口（二维码） | `/` |
| 输入 · 平板 / 大屏 | `/shuru/`、`/shuru/screen` |
| 计算 · 平板 / 大屏 | `/jisuan/`、`/jisuan/screen` |
| 输出 · 平板 / 大屏 | `/shuchu/`、`/shuchu/screen` |
| 畅想 · 平板 / 大屏 | `/changxiang/`、`/changxiang/screen` |
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
