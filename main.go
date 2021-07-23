package main

import (
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type cpuThread struct {
	cpu  int
	pid  int
	mask unix.CPUSet
}

var v = false

//cache to avoid parsing each affinity argument multiple times; probably not making things much faster, but im doing it anyway
var affCache = make(map[string]unix.CPUSet)

func main() {
	var pid = flag.Int("p", -1, "pid of main qemu process")
	var pidFile = flag.String("pidfile", "", "path to a file containing the pid of the qemu process")
	var force = flag.Bool("force", false, "ignore non fatal errors")
	var cpuAff = flag.String("k", "", "a comma separated list of vcpu-affinity pairs\n"+
		" |_ accepts ranges separated by - for both vcpu selection and affinity (ex: 0-6:8,7:3-4)\n"+
		" |_ * to select all vcpus or all affinities (ex: 3:*)\n"+
		" |_ list is processed from left to right")
	var verbose = flag.Bool("v", false, "print additional information during run")
	var ls = flag.Bool("ls", false, "list vCpu pids and affinities")
	var rootThread string
	var err error

	flag.Parse()

	if !*force && os.Getuid() != 0 {
		fmt.Println("Must be run as root to be able to set affinities")
		os.Exit(1)
	}

	if runtime.GOOS != "linux" {
		if *force {
			fmt.Println("your funeral")
			goto force
		}
		fmt.Println("Only linux is supported")
		os.Exit(1)
	}
force:

	if *verbose {
		v = true
	}

	//set pid and root thread path to avoid constantly doing string parsing and concatenation
	if *pid > 0 {
		rootThread = "/proc/" + strconv.Itoa(*pid)
	} else if *pidFile != "" {
		pFile, err := os.ReadFile(*pidFile)
		if err != nil {
			fmt.Println("Error opening PID file")
			os.Exit(1)
		}
		//sanitize input for atoi by finding first int
		firstInt(&pFile)
		*pid, err = strconv.Atoi(string(pFile))
		CrashAndBurn(err)
		rootThread = "/proc/" + string(pFile)
	} else {
		fmt.Println("Either a valid numerical PID or PID file must be given")
		os.Exit(1)
	}

	//check if pid is a valid process
	pName, err := os.ReadFile(rootThread + "/comm")
	if err != nil {
		fmt.Println("Invalid PID provided")
		os.Exit(1)
	}
	if *force || strings.Contains(string(pName), "qemu") {
		if *force {
			fmt.Println("Force flag present, not checking process name")
		}
		rootThread = "/proc/" + strconv.Itoa(*pid) + "/task/"
	} else {
		fmt.Println("process pointed to by the PID file does not seem to be a qemu process")
		os.Exit(1)
	}

	if v {
		fmt.Println("Using qemu pid: " + strconv.Itoa(*pid))
	}

	var cpus = make(map[int]cpuThread)

	dirElements, err := os.ReadDir(rootThread)
	CrashAndBurn(err)
	for _, e := range dirElements {
		//not really needed
		//if e.Name() == strconv.Itoa(*pid){
		//	continue
		//}
		if e.IsDir() {
			tName, err := os.ReadFile(rootThread + e.Name() + "/comm")
			CrashAndBurn(err)
			if strings.Contains(string(tName), "CPU") {
				var ppu cpuThread
				var g = tName
				firstInt(&g)
				ppu.cpu, err = strconv.Atoi(string(g))
				CrashAndBurn(err)
				ppu.pid, err = strconv.Atoi(e.Name())
				CrashAndBurn(err)
				err = unix.SchedGetaffinity(ppu.pid, &(ppu.mask))
				CrashAndBurn(err)
				cpus[ppu.cpu] = ppu
			}
		}
	}

	if len(cpus) == 0 {
		fmt.Println("Could not find vCpu threads. Is Qemu debug-threads on?")
		os.Exit(1)
	}

	if *ls {
		for u := 0; u < len(cpus); u++ {
			var vc = cpus[u]
			fmt.Println("vCpu " + strconv.Itoa(vc.cpu))
			fmt.Println("| PID: " + strconv.Itoa(vc.pid))
			//get all set cpus; i think the linux cpu limit is 4096
			ct := make([]int, 0)
			for i := 0; i < 4096; i++ {
				if vc.mask.IsSet(i) {
					ct = append(ct, i)
				}
			}
			fmt.Print("| Affinity: ")
			for _, i := range ct {
				fmt.Print(strconv.Itoa(i) + " ")
			}
			fmt.Print("\n")
		}
	}

	if *cpuAff != "" {
		var kv = strings.Split(*cpuAff, ",")
		for _, vs := range kv {
			if v {
				fmt.Println(vs)
			}
			var cc = strings.SplitN(vs, ":", 2)
			var ccc = parseAffinity(cc[1])
			if strings.Contains(cc[0], "*") {
				for _, i := range cpus {
					if v {
						fmt.Println("| set cpu " + strconv.Itoa(i.cpu))
					}
					err = unix.SchedSetaffinity(i.pid, &ccc)
					CrashAndBurn(err)
				}
			} else {
				for _, cpu := range numberParse(cc[0]) {
					if _, e := cpus[cpu]; !e {
						fmt.Println("Invalid cpu specified: " + strconv.Itoa(cpu))
						os.Exit(1)
					}
					if v {
						fmt.Println("| set cpu " + strconv.Itoa(cpu))
					}
					err = unix.SchedSetaffinity(cpus[cpu].pid, &ccc)
					CrashAndBurn(err)
				}
			}
		}
	} else if !*ls {
		fmt.Println("Nothing to do")
	}
}

func firstInt(p *[]byte) {
	var start = 0
	var stop int
	var prevIsNum = false
	for i := 0; i < len(*p); i++ {
		if (*p)[i] > 47 && (*p)[i] < 58 {
			if prevIsNum == false {
				start = i
				prevIsNum = true
			}
		} else {
			stop = i
			if prevIsNum {
				break
			}
		}
	}
	*p = (*p)[start:stop]
}

//take a number string and return all numbers described by it
func numberParse(j string) []int {
	var nums = make([]int, 0)
	if strings.Contains(j, "-") {
		s := strings.SplitN(j, "-", 2)
		var x, y int
		var err error
		x, err = strconv.Atoi(s[0])
		CrashAndBurn(err)
		y, err = strconv.Atoi(s[1])
		CrashAndBurn(err)
		if x == y {
			return append(nums, x)
		}
		if x > y {
			x, y = y, x
		}
		y++
		for i := x; i < y; i++ {
			nums = append(nums, i)
		}
		return nums
	} else {
		i, err := strconv.Atoi(j)
		CrashAndBurn(err)
		return append(nums, i)
	}
}

func parseAffinity(a string) unix.CPUSet {
	if vr, e := affCache[a]; e {
		if v {
			fmt.Println("| used cached value for \"" + a + "\": ")
		}
		return vr
	}
	var c unix.CPUSet
	if strings.Contains(a, "*") {
		for i := 0; i < runtime.NumCPU(); i++ {
			if v {
				fmt.Println("| aff set core " + strconv.Itoa(i))
			}
			c.Set(i)
		}
	} else {
		for _, i := range numberParse(a) {
			if v {
				fmt.Println("| aff set core " + strconv.Itoa(i))
			}
			c.Set(i)
		}
	}
	affCache[a] = c
	return c
}

func CrashAndBurn(e error) {
	if e != nil {
		fmt.Println(e)
		os.Exit(1)
	}
}
