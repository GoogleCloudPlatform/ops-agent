
function process(tag, timestamp, record)
local __field_1 = (function()
return record["logging.googleapis.com/logName"]
end)();
local v = "syslog";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/receiver_type"] = value
end)(v)
local v = __field_1;
if v == nil then v = "apache_syslog_error" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
