# qemu-cpu
qemu cpu pinning without libvirt

```
Usage of qemu-cpu:  
	-force  
		ignore non fatal errors  
	-k string  
		a comma separated list of vcpu-affinity pairs  
		 |_ accepts ranges separated by - for both vcpu selection and affinity (ex: 0-6:8,7:3-4)  
		 |_ * to select all vcpus or all affinities (ex: 3:*)  
		 |_ list is processed from left to right  
	-ls  
		list vCpu pids and affinities  
	-p int  
		pid of main qemu process (default -1)  
	-pidfile string  
		path to a file containing the pid of the qemu process  
	-v	
		print additional information during run  
