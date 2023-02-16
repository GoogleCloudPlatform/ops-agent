python - << EOF
import subprocess
import string
import datetime as dt

command = "sudo journalctl -o short-unix -u google-cloud-ops-agent*.service | grep systemd"
cmdOut = subprocess.check_output(command, shell=True)
lines = cmdOut.splitlines()
timestamps = [lines[0].split()[0], lines[-1].split()[0]]
dates_list = [ dt.datetime.fromtimestamp(float(e)) for e in timestamps]
result = dates_list[1] - dates_list[0]
milisecondsTime = result.total_seconds() * 1000
print('Restart Time : {time} ms'.format(time=milisecondsTime))
EOF
