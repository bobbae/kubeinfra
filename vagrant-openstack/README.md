# vagrant-openstack

```
vagrant up
```

in this directory brings up devstack Queens running on Ubuntu 18.04 in a vagrant virtualbox VM.

inside Vagrant file, line 9:

```
    subconfig.vm.network "public_network", ip: "192.168.1.201"
```

Change 192.168.1.201 to static IP appropriate for your environment.

After stack.sh is finished and devstack is running inside vagrant VM called devstack,
you can 

```
vagrant ssh
```

Once logged in,

```
sudo su stack -
cd
cd devstack
. openrc
env |grep OS_ > /tmp/openrc
sed -e 's/^/export /' /tmp/openrc /tmp/openrc2
echo export OS_DOMAIN_NAME=default >> /tmp/openrc2
```

Back at the host machine (where you ran vagrant up):

```
vagrant ssh devstack -- cat /tmp/openrc2 > openrc
. openrc
```

You are set to talk to openstack running inside vagrant.