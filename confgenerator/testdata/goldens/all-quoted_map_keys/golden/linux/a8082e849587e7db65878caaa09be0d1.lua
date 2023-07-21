
function process(tag, timestamp, record)
local __field_0 = (function()
return record["SYSLOG_FACILITY"]
end)();
local v = __field_0;
if v == "0" then v = "kernel"
elseif v == "1" then v = "user"
elseif v == "10" then v = "auth"
elseif v == "15" then v = "cron"
elseif v == "2" then v = "mail"
elseif v == "3" then v = "daemon"
elseif v == "4" then v = "auth"
elseif v == "5" then v = "syslog"
elseif v == "9" then v = "systemd-timesyncd"
end
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["facility"] = value
end)(v)
return 2, timestamp, record
end
