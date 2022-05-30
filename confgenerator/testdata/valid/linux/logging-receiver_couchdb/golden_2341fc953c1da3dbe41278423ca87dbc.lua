
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/couchdb";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "alert" then v = "ALERT"
elseif v == "crit" then v = "CRITICAL"
elseif v == "critical" then v = "CRITICAL"
elseif v == "debug" then v = "DEBUG"
elseif v == "emerg" then v = "EMERGENCY"
elseif v == "emergency" then v = "EMERGENCY"
elseif v == "err" then v = "ERROR"
elseif v == "error" then v = "ERROR"
elseif v == "info" then v = "INFO"
elseif v == "notice" then v = "NOTICE"
elseif v == "warn" then v = "WARNING"
elseif v == "warning" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
