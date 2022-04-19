
function process(tag, timestamp, record)
local __field_0 = (function()
return record["level"]
end)();
local v = __field_0;
if v == "DEBUG" then v = "DEBUG"
elseif v == "ERROR" then v = "ERROR"
elseif v == "INFO" then v = "INFO"
elseif v == "TRACE" then v = "TRACE"
elseif v == "WARN" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
