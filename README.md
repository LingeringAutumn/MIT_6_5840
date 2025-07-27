# MIT 6.5840 - Lab 1: MapReduce 实现

本项目是对 [MIT 6.5840](https://pdos.csail.mit.edu/6.5840/) 分布式系统课程中 Lab1：MapReduce 的完整实现。
该实验的目标是构建一个简化版的 MapReduce 分布式框架，包括任务调度、容错处理、Worker 执行等核心逻辑。

---

## 📁 项目结构说明

```
.
├── main/
│   ├── mrcoordinator.go   # 启动 Coordinator 主节点
│   ├── mrsequential.go    # 顺序调试入口（用于测试 mapf/reducef）
│   └── mrworker.go        # 启动 Worker 节点
├── mr/
│   ├── rpc.go             # RPC 协议定义
│   ├── worker.go          # Worker 执行逻辑
│   └── coordinator.go     # Coordinator 逻辑核心
├── pg-*                   # 测试输出文件
├── mr-*                   # 中间输出文件
└── docs/                  # 📚 课程资料、实验笔记等
```

---

## 🚀 运行方式

```bash
  cd src/main
  bash test-mr.sh 
```

---

## 🧠 实验目标

* 掌握 MapReduce 编程模型
* 实现一个简化的 MapReduce 框架
* 理解 RPC、任务调度、并发控制、容错恢复等分布式系统核心问题

---

## 📄 文档资料（Documentation）

你可以在 [`/docs`](./docs/) 目录下查看以下内容：

* 🧪 [实验要求文档](./docs/west2online_lab1.md)
* 📓 [个人实验笔记](./docs/note_lab1.md)
* 🔗 [MIT 6.5840 课程网页](https://pdos.csail.mit.edu/6.5840/)

---
