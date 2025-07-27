package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

// Add your RPC definitions here.

type RequestTaskArgs struct {
	WorkerId int
}
type RequestTaskReply struct {
	TaskType string // master传递的"map","reduce","wait","exit"
	TaskId   int
	NReduce  int
	NMap     int
	Filename string
	Files    []string
}

type ReportTaskArgs struct {
	TaskType string // "map" 或 "reduce"
	TaskID   int    // 完成的任务编号
}

type ReportTaskReply struct {
	Success bool // 表示接收成功
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
