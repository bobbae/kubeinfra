# vagrant kubernetes

Vagrantfile here creates multiple VMs. One for kubernetes master. Two for worker nodes. 
Additional nodes can be added easily by changing (1..2) to (1..N) in the file.

First time use:
```
vagrant up
```

This one command runs kubeadm init on the master node.  And kubectl join in the worker nodes, named node1 and node2.  

You can check that cluster is up by checking the nodes on the master node:
```
$ vagrant ssh master
vagrant@master:~$ sudo su -
root@master:~# export KUBECONFIG=/etc/kubernetes/admin.conf
root@master:~# kubectl get nodes
NAME      STATUS    ROLES     AGE       VERSION
master    Ready     master    3m        v1.10.2
node1     Ready     <none>    1m        v1.10.2
node2     Ready     <none>    19s       v1.10.2
```

Shutdown by doing
```
$ vagrant halt -f master node1 node2
```


