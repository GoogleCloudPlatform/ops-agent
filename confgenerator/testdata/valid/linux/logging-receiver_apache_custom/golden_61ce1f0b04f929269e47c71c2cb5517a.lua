
function process(tag, timestamp, record)
local __field_0 = (function()
return record["level"]
end)();
local v = __field_0;
if v == "alert" then v = "ALERT"
elseif v == "crit" then v = "CRITICAL"
elseif v == "debug" then v = "DEBUG"
elseif v == "emerg" then v = "EMERGENCY"
elseif v == "error" then v = "ERROR"
elseif v == "info" then v = "INFO"
elseif v == "notice" then v = "NOTICE"
elseif v == "trace1" then v = "DEBUG"
elseif v == "trace2" then v = "DEBUG"
elseif v == "trace3" then v = "DEBUG"
elseif v == "trace4" then v = "DEBUG"
elseif v == "trace5" then v = "DEBUG"
elseif v == "trace6" then v = "DEBUG"
elseif v == "trace7" then v = "DEBUG"
elseif v == "trace8" then v = "DEBUG"
elseif v == "warn" then v = "WARNING"
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
