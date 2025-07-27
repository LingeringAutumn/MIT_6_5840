1. 概述
   https://github.com/LingeringAutumn/MIT_6_5840
   [图片]
1. 根据我对代码和这篇论文pdos.csail.mit.edu的理解，这个实验做的是一个分布式的MapReduce系统，简单来说，就是让很多台机器对一个大任务进行拆分，由一个master和多个worker共同协作完成这个任务，每个worker分别进行计算，然后去重（根据论文的说法，去重是在每个worker里进行完毕之后再进行数据汇总，这样能大大减少传输数据的过程中对网络的消耗，提高性能），最后进行统计。
   [图片]
2. 我所做的部分就是对mr目录下的三个组件进行完善
- Coordinator（协调者）
- Worker（工作者，任务的实际执行者）
- RPC （定义数据结构，用于通信）
  其中，Coordinator 负责分配任务、记录状态、处理 worker 上报的任务完成信息，以及检测任务超时重试。
  Worker 通过 RPC 向 Coordinator 请求任务，执行 Map 或 Reduce 函数，并上报结果。
2. 实际工作
1. Coordinator
1. 数据结构设计
   type Task struct {
   TaskId    int
   FileName  string
   State     TaskState // Idle, InProgress, Completed
   StartTime time.Time // 为任务超时重试设计
   }

type Coordinator struct {
files       []string
nReduce     int
mapTasks    []Task
reduceTasks []Task
phase       string       // 当前阶段："map" / "reduce" / "done"
taskLock    sync.Mutex   // 用于保护共享状态
}
2. 任务调度逻辑
   func (c *Coordinator) RequestTask(args *RequestTaskArgs, reply *RequestTaskReply) error {
   c.taskLock.Lock()
   defer c.taskLock.Unlock()

        if c.phase == "map" {
                // 分配空闲的 map 任务
                for i, task := range c.mapTasks {
                        if task.State == Idle {
                                c.mapTasks[i].State = InProgress
                                // 下面这一行卡了很久
                                c.mapTasks[i].StartTime = time.Now()
                                reply.TaskType = "map"
                                reply.TaskId = task.TaskId
                                reply.Filename = task.FileName
                                reply.NReduce = c.nReduce
                                reply.NMap = len(c.mapTasks)
                                return nil
                        }
                }
                // 若 map 已全部完成，进入 reduce 阶段
                if c.allMapDone() {
                        c.phase = "reduce"
                } else {
                        reply.TaskType = "wait"
                        return nil
                }
        }
        // 类似地处理 reduce 任务
        ...
}

值得一提的是，c.mapTasks[i].StartTime = time.Now()这一行折磨了我很久，一开始我这里没写数组，一直报错；后面好不容易定位到这个地方修改完之后，还是报错，又因为修改过了所以一直没检查这里，最后才发现是把map模块的代码复制到这下面的时候，忘记把map改成reduce了...
3.任务超时重置（这个是用来通过最后的crash test的）
func (c *Coordinator) MonitorTaskTimeouts() {
for {
time.Sleep(500 * time.Millisecond) // 每 0.5 秒扫描一次

       c.taskLock.Lock()
       tasks := []*[]Task{&c.mapTasks, &c.reduceTasks}
       for _, taskList := range tasks {
          for i := range *taskList {
             task := &(*taskList)[i]
             if task.State == InProgress {
                if time.Since(task.StartTime) > 10*time.Second {
                   fmt.Printf("[Coordinator] Task %d timeout, resetting to Idle\n", task.TaskId)
                   task.State = Idle
                   task.StartTime = time.Time{} // 清空时间
                }
             }
          }
       }
       c.taskLock.Unlock()
    }
}
2. Worker
   Worker 主体是一个循环，持续向 Coordinator 请求任务，根据任务类型执行不同处理流程。
   核心函数如下。
   // main/mrworker.go calls this function.
   func Worker(mapf func(string, string) []KeyValue,
   reducef func(string, []string) string) {

   for {
   args := RequestTaskArgs{WorkerId: os.Getpid()}
   reply := RequestTaskReply{}

       ok := call("Coordinator.RequestTask", &args, &reply)
       if !ok {
          break
       }

       switch reply.TaskType {
       case "map":
          content, err := os.ReadFile(reply.Filename)
          if err != nil {
             log.Fatalf("[Worker %d] cannot read file: %v", args.WorkerId, err)
          }
          kva := mapf(reply.Filename, string(content))
          bucket := make([][]KeyValue, reply.NReduce)

          for _, kv := range kva {
             r := ihash(kv.Key) % reply.NReduce
             bucket[r] = append(bucket[r], kv)
          }

          for r, bucket := range bucket {
             tempFilename := fmt.Sprintf("mr-%d-%d", reply.TaskId, r)
             file, err := os.Create(tempFilename)
             if err != nil {
                log.Fatalf("[Worker %d] cannot create middle file: %v", reply.TaskId, err)
             }
             enc := json.NewEncoder(file)
             for _, kv := range bucket {
                err := enc.Encode(&kv)
                if err != nil {
                   log.Fatalf("[Worker %d] encode midddle file error: %v", reply.TaskId, err)
                }
             }
             file.Close()
          }

          reportArgs := ReportTaskArgs{
             TaskType: "map",
             TaskID:   reply.TaskId,
          }
          reportReply := ReportTaskReply{}
          _ = call("Coordinator.ReportTaskDone", &reportArgs, &reportReply)

       case "reduce":
          intermediate := []KeyValue{}
          for m := 0; m < reply.NMap; m++ {
             filename := fmt.Sprintf("mr-%d-%d", m, reply.TaskId)
             file, err := os.Open(filename)
             if err != nil {
                log.Fatalf("[Worker %d] 打开中间文件失败: %s\n", args.WorkerId, filename)
             }
             dec := json.NewDecoder(file)
             for {
                var kv KeyValue
                if err := dec.Decode(&kv); err != nil {
                   break // EOF
                }
                intermediate = append(intermediate, kv)
             }
             file.Close()
          }

          sort.Slice(intermediate, func(i, j int) bool {
             return intermediate[i].Key < intermediate[j].Key
          })

          // 原子写入输出文件
          tempFile, err := os.CreateTemp(".", fmt.Sprintf("mr-out-%d-tmp", reply.TaskId))
          if err != nil {
             log.Fatalf("[Worker %d] cannot create temp file: %v", args.WorkerId, err)
          }
          enc := bufio.NewWriter(tempFile)

          i := 0
          for i < len(intermediate) {
             j := i + 1
             for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
                j++
             }
             values := []string{}
             for k := i; k < j; k++ {
                values = append(values, intermediate[k].Value)
             }
             output := reducef(intermediate[i].Key, values)
             fmt.Fprintf(enc, "%v %v\n", intermediate[i].Key, output)
             i = j
          }
          enc.Flush()
          tempFile.Close()

          // 原子替换输出
          finalOutput := fmt.Sprintf("mr-out-%d", reply.TaskId)
          err = os.Rename(tempFile.Name(), finalOutput)
          if err != nil {
             log.Fatalf("[Worker %d] failed to rename temp file: %v\n", args.WorkerId, err)
          }
          fmt.Printf("[Worker %d] REDUCE task %d write complete\n", args.WorkerId, reply.TaskId)

          reportArgs := ReportTaskArgs{
             TaskType: "reduce",
             TaskID:   reply.TaskId,
          }
          reportReply := ReportTaskReply{}
          _ = call("Coordinator.ReportTaskDone", &reportArgs, &reportReply)

       case "wait":
          time.Sleep(1 * time.Second)

       case "exit":
          return

       default:
          time.Sleep(1 * time.Second)
       }
   }
   }
3. RPC
   定义了一些结构体。
   type RequestTaskArgs struct {
   WorkerId int
   }
   type RequestTaskReply struct {
   TaskType string
   TaskId   int
   NReduce  int
   NMap     int
   Filename string
   }
   type ReportTaskArgs struct {
   TaskType string
   TaskID   int
   }
   type ReportTaskReply struct {
   Success bool
   }

3. 执行流程图
   我语言描述，然后让ai帮我画了一下
1. 整体结构
   +------------------+
   |     Worker       |
   | 请求任务 Request |
   +------------------+
   |
   v
   +---------------------+       若有空闲任务        +---------------------+
   |     Coordinator     | -----------------------> |   分配 Task 给 Worker |
   | 保存任务状态、监控  |                           +---------------------+
   +---------------------+
   ^
   |
   |  完成上报 Report
   +------------------+
   |     Worker       |
   +------------------+
2. 执行流程
   [任务执行流程图]

                   +---------+
                   |  Mapf   |
                   +---------+
                        |
         +--------------+--------------+
         |                             |
   写入中间文件                 ihash(key) 分桶
   mr-X-Y 格式                  Y 为 ReduceId

                   +-----------+
                   | Reduce 阶段 |
                   +-----------+
                        |
              读取对应中间文件
                        |
                  分组、排序
                        |
                    执行 Reducef
                        |
                    写入 mr-out-Y
4. 踩坑
1. 最初没有设置任务超时机制，导致 crash 测试失败。
1. 最开始我的结构体里面是没有时间的，然后crash一直不过，一开始以为是有什么细节的bug；后面看了一下crash的测试的指令，再去看了一下论文，然后又问了ai，再仔细想了一下，发现不对——按最初的方案，不加一个时间，那么，如果某个 Worker 卡住了任务，Coordinator 永远不会重试。
2. 后面是这样解决的，然后就全过了
1. 每个任务结构体里加入 StartTime 字段：
   StartTime time.Time
   2.启动后台协程，定期扫描任务，超过一定时间就将状态重置为 Idle
   if time.Since(task.StartTime) > 10*time.Second {
   task.State = Idle
   }

2. 剩下的坑没啥了，都是简单的问题，多看一下就解决了
