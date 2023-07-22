import subprocess
import re
import os

# step 1: get ovs-dpctl show out to make sure which ports are being used
try:
    ovsDPCtlShow = subprocess.Popen(['ovs-dpctl', 'show'],
                            stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT)
except subprocess.CalledProcessError:
    print("failed to execute ovs-dpctl show command")
    os.Exit(1)
    
stdout = ovsDPCtlShow.communicate()

usedPortList = re.findall("port (\d+)", str(stdout))

# Step 2: Check ovs flows dumps
try:
    ovsDumpFlows = subprocess.Popen(['ovs-ofctl', 'dump-flows', 'azure0'],
                            stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT)
except subprocess.CalledProcessError:
    print("failed to execute ovs-ofctl dump-flows command")
    os.Exit(1)
    
stdout = ovsDumpFlows.communicate()
allPortList = re.findall("in_port=(\d+)", str(stdout))

unUsedPortList = []
for port in allPortList:
    if port not in usedPortList:
        unUsedPortList.append(port)

# Step 3: delete leaked rules
# only use unused ports
for port in unUsedPortList:
    deleteCommand = "ovs-ofctl del-flows azure0 ip,in_port=%s"%port
    try:
        os.system(deleteCommand)
    except:
        print("delete command %s does not work"%deleteCommand)
        os.Exit(1)