package mr

import (
	"fmt"
	"log"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

type TaskState int

const (
	Idle TaskState = iota
	InProgress
	Completed
)

type Task struct {
	TaskId    int
	FileName  string // Map 任务对应的输入文件
	State     TaskState
	StartTime time.Time // 记录任务被分配的时间，如果不加这个最后一个测试过不了
}

type Coordinator struct {
	// Your definitions here.
	files       []string   // 所有 map 输入文件
	nReduce     int        // reduce 数量
	mapTasks    []Task     // 所有 map 任务状态
	reduceTasks []Task     // 所有 reduce 任务状态
	phase       string     // 当前阶段："map" / "reduce" / "done"
	taskLock    sync.Mutex // 	多个 Worker 并发请求任务，要加锁保护任务状态
}

// Your code here -- RPC handlers for the worker to call.
func (c *Coordinator) ReportTaskDone(args *ReportTaskArgs, reply *ReportTaskReply) error {
	c.taskLock.Lock()
	defer c.taskLock.Unlock()

	switch args.TaskType {
	case "map":
		if args.TaskID < 0 || args.TaskID >= len(c.mapTasks) {
			reply.Success = false
			return nil
		}
		if c.mapTasks[args.TaskID].State == Completed {
			reply.Success = true
			return nil
		}
		c.mapTasks[args.TaskID].State = Completed
		reply.Success = true
		// fmt.Printf("[Master] MAP task %d has finished\n", args.TaskID)

	case "reduce":
		if args.TaskID < 0 || args.TaskID >= len(c.reduceTasks) {
			reply.Success = false
			return nil
		}
		if c.reduceTasks[args.TaskID].State == Completed {
			reply.Success = true
			return nil
		}
		c.reduceTasks[args.TaskID].State = Completed
		reply.Success = true
		// fmt.Printf("[Master] REDUCE task %d has finished\n", args.TaskID)
	default:
		reply.Success = false
	}

	return nil
}

func (c *Coordinator) RequestTask(args *RequestTaskArgs, reply *RequestTaskReply) error {
	c.taskLock.Lock()
	defer c.taskLock.Unlock()

	if c.phase == "map" {
		for i, task := range c.mapTasks {
			if task.State == Idle {
				// 找到一个空闲 MAP 任务，分配给 worker
				c.mapTasks[i].State = InProgress
				//task.StartTime = time.Now()
				c.mapTasks[i].StartTime = time.Now()

				reply.TaskType = "map"
				reply.TaskId = task.TaskId
				reply.Filename = task.FileName
				reply.NReduce = c.nReduce
				reply.NMap = len(c.mapTasks)
				// fmt.Printf("[Master] allocate MAP task %d to worker\n", task.TaskId)
				return nil
			}
		}

		// 没有空闲任务，检查是否进入 reduce 阶段
		if c.allMapDone() {
			//c.initReduceTasks()
			c.phase = "reduce"
		} else {
			reply.TaskType = "wait"
			return nil
		}
	}

	if c.phase == "reduce" {
		for i, task := range c.reduceTasks {
			if task.State == Idle {
				c.reduceTasks[i].State = InProgress
				//task.StartTime = time.Now()
				c.reduceTasks[i].StartTime = time.Now()

				reply.TaskType = "reduce"
				reply.TaskId = task.TaskId
				reply.NReduce = c.nReduce
				reply.NMap = len(c.mapTasks)
				// fmt.Printf("[Master] allocate MAP task %d to worker\n", task.TaskId)
				return nil
			}
		}

		// 没有空闲 reduce 任务，检查是否全部完成
		if c.allReduceDone() {
			c.phase = "done"
			reply.TaskType = "exit"
			return nil
		} else {
			reply.TaskType = "wait"
			return nil
		}
	}

	if c.phase == "done" {
		reply.TaskType = "exit"
		return nil
	}

	reply.TaskType = "wait"
	return nil
}

func (c *Coordinator) allMapDone() bool {
	for _, task := range c.mapTasks {
		if task.State != Completed {
			return false
		}
	}
	return true
}
func (c *Coordinator) allReduceDone() bool {
	for _, task := range c.reduceTasks {
		if task.State != Completed {
			return false
		}
	}
	return true
}

// 初始化 reduce 任务
func (c *Coordinator) initReduceTasks() {
	// 要清空一下不然会出错
	c.reduceTasks = make([]Task, c.nReduce)
	for i := 0; i < c.nReduce; i++ {
		c.reduceTasks[i] = Task{
			TaskId:    i,
			State:     Idle,
			StartTime: time.Time{},
		}
	}
}

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	c.taskLock.Lock()
	defer c.taskLock.Unlock()

	if c.phase != "reduce" {
		return false
	}

	for _, task := range c.reduceTasks {
		if task.State != Completed {
			return false
		}
	}

	c.phase = "done"
	// fmt.Println("[Master] all REDUCE task has finished, over!")
	return true
}

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

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{
		mapTasks: []Task{},
		files:    files,
		nReduce:  nReduce,
		phase:    "map",
	}

	// 初始化 map 任务
	for i, file := range files {
		c.mapTasks = append(c.mapTasks, Task{
			TaskId:    i,
			FileName:  file,
			State:     Idle,
			StartTime: time.Time{}, // 初始为零时间，表示还未开始
		})
	}

	// 初始化 reduce 任务
	c.initReduceTasks()

	// 启动后台协程监控任务超时
	go c.MonitorTaskTimeouts()

	// 启动 RPC 服务
	c.server()

	return &c
}
