import subprocess
import os
import re
import requests

# step 1: get all ovs bridges:
try:
    ovsBridgeShow = subprocess.Popen(['ovs-vsctl', 'list-br'],
                                     stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT)
except subprocess.CalledProcessError:
    print("failed to execute ovs-vsctl show command")
    os.Exit(1)

stdout = ovsBridgeShow.communicate()
ovsBridgeList = stdout[0].decode("utf-8".strip()).split('\n')

# step 2: remove all ovs bridges
for bridge in ovsBridgeList:
    if bridge != "":
        deleteCommand = "ovs-vsctl del-br %s"%bridge
        try:
            print("deleting ovs bridge by: ", deleteCommand)
            os.system(deleteCommand)
        except:
            print("failed to delete all ovs bridges")

# step 3: reset vSwitch configuration to clean state and delete manager
try:
    os.system("ovs-vsctl del-manager")
    os.system("ovs-vsctl emer-reset")
except:
    print("failed to reset vSwitch configuration and delete manager")

# step 4: check if ovs flows exist anymore
try:
    ovsDPCtlShow = subprocess.Popen(['ovs-dpctl', 'show'],
                            stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT)
except subprocess.CalledProcessError:
    print("failed to execute ovs-dpctl show command")
    os.Exit(1)
    
stdout = ovsDPCtlShow.communicate()
if stdout[0].decode("utf-8") != "":
    print("ovs flows still exist, please check if all ovs bridges are removed from system")
    os.Exit(1)

# step 5: delete cni state file:
cniStatePath = "/var/run/azure-vnet.json"
if os.path.exists(cniStatePath):
    try:
        os.system("rm /var/run/azure-vnet.json")
    except:
        print("failed to delete cni state file")
        os.Exit(1)

# step 6: delete az* interfaces as supporting for apipa connectivity
try:
    ovsBridgeShow = subprocess.Popen(['ls', '/sys/class/net'],
                                     stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT)
except subprocess.CalledProcessError:
    print("failed to execute get all interfaces command")
    os.Exit(1)

stdout = ovsBridgeShow.communicate()
for interface in stdout[0].decode("utf-8").split('\n'):
    if interface.startswith("az"):
        try:
            ovsBridgeShow = subprocess.Popen(['ip', 'link', 'delete', interface],
                                     stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT)
        except subprocess.CalledProcessError:
            print("failed to delete interface: ", interface)
            os.Exit(1)

# step 7: check internet connectivity after ovs bridges are removed
url = "http://www.bing.com"
timeout = 5
try:
	request = requests.get(url, timeout=timeout)
	print("Connected to the Internet")
except (requests.ConnectionError, requests.Timeout) as exception:
	print("No internet connection.")