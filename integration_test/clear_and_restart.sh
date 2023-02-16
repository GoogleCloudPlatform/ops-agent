sudo journalctl --rotate
sudo journalctl --vacuum-time=1s
sudo systemctl restart google-cloud-ops-agent.service

sudo journalctl --rotate;sudo journalctl --vacuum-time=1s; sudo systemctl restart google-cloud-ops-agent.service