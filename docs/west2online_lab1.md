# Golang Lab7

## 目的

- 掌握分布式系统设计与实现的重要原则和关键技术

- 学习和实现MapReduce

- 学习和实现Raft算法

## 任务

6.824（2023年后改名6.5840）包括4个编程作业

- 6.5840 Lab 1: MapReduce

- 6.5840 Lab 2: Key/Value Server

- 6.5840 Lab 3: Raft

- 6.5840 Lab 4: Fault-tolerant Key/Value Service

总课程表如下，里面包含了所有相关论文和作业（当然，纯英文的）

https://pdos.csail.mit.edu/6.824/schedule.html

通过git获取实验初始的框架

```bash
git clone git://g.csail.mit.edu/6.5840-golabs-2024 6.5840
```

- 6.824的lab不能在Windows下运行（WSL按照文档说明，无法正常运行）
- 你的IDE可能会报很多错，这是正常的，它可以跑起来
- 每个lab都有对应的测试脚本或代码，你可以从这些文件入手



本次作业，你只需要完成 Lab1，后续的3个Lab以周会的形式进行

## Lab1

**`因为lab的内容并不好理解，以下内容旨在帮助你找到一个相对合理的主线去理解整个lab，但这并不代表你可以不去看原文（可以用GPT翻译），整个作业中充斥着大量的小细节，而这些细节本文无法涵盖，而它们可能会让你疑惑很久`**

在这个实验中，你将构建一个MapReduce系统用于计算多个txt文件的单词计数

### MapReduce简介

我相信你不会想看又臭又长的英文论文的，所以这里我给出一些核心概念的解释

MapReduce 的名称来源于其两个主要步骤：Map 和 Reduce。

1. **Map 步骤**：
- 输入数据被分割成若干小块（通常是键值对）。
- 每个小块数据被传递给一个 Map 函数进行处理。
- Map 函数生成一组中间结果（键值对）。
2. **Shuffle 和 Sort 步骤（隐式）**：
- 中间结果根据键进行分组和排序，以便相同键的数据能被传递到同一个 Reduce 函数。
- 这个步骤通常由框架自动处理，不需要用户显式编写代码。
3. **Reduce 步骤**：
- 每个 Reduce 函数接收来自 Map 步骤的中间结果，并进行汇总、聚合或其他计算。
- Reduce 函数生成最终的输出结果。

### 单体式实现

6.5840在`src/main/mrsequential.go`中提供了单体式的MapReduce实现

```Shell
$ cd ~/6.5840
$ cd src/main
$ go build -buildmode=plugin ../mrapps/wc.go #编译插件
$ rm mr-out*
$ go run mrsequential.go wc.so pg*.txt
$ more mr-out-0
A 509
ABOUT 2
ACT 8
...
```

#### 使用Plugin加载Map函数和Reduce函数

`src/mrapps/wc.go `中定义了Map函数和Reduce函数，逻辑也很简单

Map函数为每个单词生成了一个key为单词内容，value为1的键值对

```Go
func Map(filename string, contents string) []mr.KeyValue {
    // function to detect word separators.
    // 定义字符分隔函数
    ff := func(r rune) bool { return !unicode.IsLetter(r) }

    // split contents into an array of words.
    // 分割内容成单词数组
    words := strings.FieldsFunc(contents, ff)
    // 遍历每个单词，为每个单词生成一个键值对 mr.KeyValue{w, "1"} “1”表示这个单词出现过一次
    kva := []mr.KeyValue{}
    for _, w := range words {
        kv := mr.KeyValue{w, "1"}
        kva = append(kva, kv)
    }
    return kva
}

func Reduce(key string, values []string) string {
    // return the number of occurrences of this word.
    // 计算键的出现次数
    return strconv.Itoa(len(values))
}
```

在运行单体式实例时，我们将这个文件编译成plugin

```Go
go build -buildmode=plugin ../mrapps/wc.go
```

然后在单体式MapReduce运行时加载它们，也就是mapf和reducef，它们的本质就是函数变量

```Go
func main() {
    if len(os.Args) < 3 {
        fmt.Fprintf(os.Stderr, "Usage: mrsequential xxx.so inputfiles...\n")
        os.Exit(1)
    }

    mapf, reducef := loadPlugin(os.Args[1])
    ...
}

func loadPlugin(filename string) (func(string, string) []mr.KeyValue, func(string, []string) string) {
    p, err := plugin.Open(filename)
    if err != nil {
        log.Fatalf("cannot load plugin %v", filename)
    }
    xmapf, err := p.Lookup("Map")
    if err != nil {
        log.Fatalf("cannot find Map in %v", filename)
    }
    mapf := xmapf.(func(string, string) []mr.KeyValue) // 类型断言
    xreducef, err := p.Lookup("Reduce")
    if err != nil {
        log.Fatalf("cannot find Reduce in %v", filename)
    }
    reducef := xreducef.(func(string, []string) string) // 类型断言

    return mapf, reducef
}
```

#### main函数分别用Map函数和Reduce函数做了什么

- 遍历每个txt文件进行Map，将获得的key value 切片合并
- 对key value 切片进行排序，方便计数
- 将相同key的key value进行合并进行Reduce，然后输出

你可能会觉得Reduce好像没干什么事情，在单体模式下，确实，但在分布式系统中，Reduce的作用就能体现出来了

```Go
// for sorting by key.
type ByKey []mr.KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintf(os.Stderr, "Usage: mrsequential xxx.so inputfiles...\n")
        os.Exit(1)
    }

    mapf, reducef := loadPlugin(os.Args[1])
    // 遍历每个txt文件
    intermediate := []mr.KeyValue{}
    for _, filename := range os.Args[2:] {
        // 对每个txt文件进行Map，将获得的key value 切片合并
        file, err := os.Open(filename)
        if err != nil {
            log.Fatalf("cannot open %v", filename)
        }
        content, err := ioutil.ReadAll(file)
        if err != nil {
            log.Fatalf("cannot read %v", filename)
        }
        file.Close()
        kva := mapf(filename, string(content))
        intermediate = append(intermediate, kva...)
    }

    //
    // a big difference from real MapReduce is that all the
    // intermediate data is in one place, intermediate[],
    // rather than being partitioned into NxM buckets.
    //
    
    // 按照key，也就是单词进行排序，将相同的单词聚集在一起
    sort.Sort(ByKey(intermediate))
    
    // 创建输出文件
    oname := "mr-out-0"
    ofile, _ := os.Create(oname)

    //
    // call Reduce on each distinct key in intermediate[],
    // and print the result to mr-out-0.
    //
    i := 0
    for i < len(intermediate) {
        // 对相同的单词进行计数，保存到values切片，再进行Reduce
        j := i + 1
        for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
            j++
        }
        values := []string{}
        for k := i; k < j; k++ {
            values = append(values, intermediate[k].Value)
        }
        output := reducef(intermediate[i].Key, values)

        // this is the correct format for each line of Reduce output.
        fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)

        i = j
    }

    ofile.Close()
}
```

#### 单体式较分布式省略了什么步骤

单单看单体式的实现并不能帮助你理解分布式系统是怎么运作的，单体式实现省略了一些关键的东西

首先你要记住：分布式拥有多个节点同时进行工作

1. 单体式直接遍历了每个txt文件进行map任务，但是分布式的时候如何进行map任务的划分和分配？
2. 单体式直接把Map后的中间结果临时保存在了一个切片内，但是分布式显然不能这么做，分布式系统通过Map产生的中间结果一定不能相互干扰，
3. 单体式通过一个比较巧妙的循环分割了reduce任务，分布式的reduce任务又应该怎么划分？
4. 分布式不同节点之间是怎么通信的？

你肯定是一头雾水，别急，继续往下看

### 你的任务

当然，你的实现必须是分布式的，包括1个Coordinator（协调器）和多个Worker（工作节点）

其中,Coordinator的启动入口在`main/mrcoordinator.go`

Worker的启动入口在`main/mrworker.go` （需要插件）

MapReduce 系统通过分布式文件系统（DFS）来管理和存储数据，在这个lab中，**你可以默认所有节点共享当前目录的所有txt文件**

#### Coordinator需要做什么

1. <u>将Map任务和Reduce任务的分解成多个小任务</u>

Map任务的分解比较简单，因为节点共享所有txt文件，你可以直接把Map任务通过文件名划分，Worker只需要拿到对应的文件名就可以开始工作了

关于Reduce任务的分解，lab1在`mr/worker.go`给出了一个关键的函数

```Go
// 关键是注释
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
    h := fnv.New32a()
    h.Write([]byte(key))
    return int(h.Sum32() & 0x7fffffff)
}
```

首先，lab1规定了一个nReduce参数，代表着Reduce任务的数量，同时，每个Map任务都需要为Recude任务创建nRecude个中间文件，我们约定一个合理的中间文件是`mr-X-Y`，其中X是Map任务号，Y是reduce任务号。

就像注释所说的那样，我们可以通过ihash(key)来决定Y的值，将中间键/值写入文件（lab1的推荐使用Go的`encoding/json`包写文件）

所以，我们可以在Map阶段结束后，通过检查当前目录下文件的文件名，整合出具有相同Y值的文件名作为一个Reduce任务

2. <u>Worker会向Coordinator请求任务，Coordinator需要将分解的小任务分配出去</u>

需要注意的点

- 同时有多个节点向Coordinator请求任务，怎么保证任务不会被重复分配（答案是加合适的互斥锁）？
- 我们不能保证Worker是可靠的，如果Worker崩了，Coordinator需要再次把任务分配出去，怎么实现（对每个任务进行超时检查）？

3. <u>Coordinator需要**知道**并且**告诉Worker**现在进行到了程序的哪个阶段（Map，Reduce还是已经结束？怎么切换阶段才是合理的？）</u>

#### Worker需要做什么

<u>不断向Coordinator请求任务，直到所有任务已完成</u>

需要注意的点

- Map和Recude的逻辑从哪里加载？它们究竟在做什么？
- Worker怎么知道所有任务已经结束，可以退出了？

### 分布式节点之间的通信

这里我们只介绍通信的方法，其底层实现比较复杂，欢迎同学们研究

在`mr`文件夹的3个文件中有以下例子

Work可以通过call方法，传入`Coordinator.方法名`，对应的`Args`和`Reply`进行通信

**注意**，`RPC`仅发送名称以大写字母开头的结构字段。子结构也必须有大写的字段名称。

```Go
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
    reply.Y = args.X + 1
    return nil
}
type ExampleArgs struct {
    X int
}

type ExampleReply struct {
    Y int
}
//
// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
//
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

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
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
```

### 你可以也应该修改的文件

1. `mr/coordinator.go `

这里是你的Coordinator的实现

你需要完成

- Coordinator结构体的定义（Coordinator struct）和初始化（MakeCoordinator）
- 仿造Example函数定义你需要的RPC handler供Worker调用

2. `mr/rpc.go `

这里你应该添加你的RPC handler的 Args 和 Reply 的定义，就像Example那样

3. `mr/worker.go `

这里是你的Worker实现

你需要完成

- Worker函数，每一个Worker的都会执行这个函数，一个基本思路是在函数中开启循环向Coordinator获取任务
- RPC handler的Call函数，就像CallExample()那样

### 关于测试脚本

lab1提供了一个测试脚本在`main/test-mr.sh`中。测试检查`wc`和`indexer` MapReduce应用程序在给定`pg-xxx.txt`文件作为输入时是否生成正确的输出。测试还检查你的实现是否并行运行Map和Reduce任务，以及你的实现是否能够从崩溃的工作进程中恢复。

如果你现在运行测试脚本，它将挂起，因为协调器从未完成：

```Bash
bash
复制代码
$ cd ~/6.5840/src/main
$ bash test-mr.sh
*** Starting wc test.
```

你可以将`mr/coordinator.go`中的`Done`函数中的`ret := false`改为`true`，这样协调器会立即退出。然后：

测试脚本期望在名为`mr-out-X`的文件中看到输出，每个Reduce任务一个文件。`mr/coordinator.go`和`mr/worker.go`的空实现没有生成这些文件（或者做其他事情），所以测试失败。

当你完成后，测试脚本的输出应如下所示：

你可能会看到一些来自Go RPC包的错误信息，看起来像这样：

```CSS
2019/12/16 13:27:09 rpc.Register: method "Done" has 1 input parameters; needs exactly three
```

忽略这些消息；将协调器注册为RPC服务器时，会检查所有方法是否适合用于RPC（有3个输入参数）；我们知道`Done`不是通过RPC调用的。

**`理解测试脚本的逻辑对你理解整个lab1很有帮助，你可以通过GPT等工具详细了解其逻辑`**
