# ovs_scripts

`remove_ovs_bridges.py` script is for removing ovs switch(azure0) and the and openflow rules configured with it 
ssh connection will not be lost when running script
It will get all existing ovs bridges and delete them and then delete CNI state file 
and all interfaces starting with `az` that are used for supporting apipa connectivity. After that,
it will bring back VM to original state with eth0 as primary interface and 
check if Linux VM internet connectivity is still working.

`remove_ovs_leaked_rules.py` script is for removeing all leaked ovs rules
It will check ovs flow dumps and filter which ports are being used. Then delete these ovs rules that
are not associated with used ports.

To run these script, clone scripts to Linux VM with ovs and have Python3 environment ready:
paulyu@paul-microsoft:~$ which python3
/usr/bin/python3

Run script:
python3 remove_ovs_bridges.py
python3 remove_ovs_leaked_rules.py
