
function process(tag, timestamp, record)
local __field_0 = (function()
return record["logging.googleapis.com/logName"]
end)();
local v = __field_0;
if v == nil then v = "mysql_syslog_slow" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
