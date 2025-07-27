package mr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

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

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
