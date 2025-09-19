
function process(tag, timestamp, record)
local __field_0 = (function()
if record["log"] == nil
then
return nil
end
return record["log"]["level"]
end)();
local v = __field_0;
if v == "CRITICAL" then v = "ERROR"
elseif v == "DEBUG" then v = "DEBUG"
elseif v == "DEPRECATION" then v = "WARNING"
elseif v == "ERROR" then v = "ERROR"
elseif v == "FATAL" then v = "FATAL"
elseif v == "INFO" then v = "INFO"
elseif v == "TRACE" then v = "DEBUG"
elseif v == "WARN" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
