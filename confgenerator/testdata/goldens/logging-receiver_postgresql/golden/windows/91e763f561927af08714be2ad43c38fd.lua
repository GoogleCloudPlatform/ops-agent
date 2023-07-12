
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/postgresql_general";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "DEBUG1" then v = "DEBUG"
elseif v == "DEBUG2" then v = "DEBUG"
elseif v == "DEBUG3" then v = "DEBUG"
elseif v == "DEBUG4" then v = "DEBUG"
elseif v == "DEBUG5" then v = "DEBUG"
elseif v == "DETAIL" then v = "DEBUG"
elseif v == "ERROR" then v = "ERROR"
elseif v == "FATAL" then v = "CRITICAL"
elseif v == "INFO" then v = "INFO"
elseif v == "LOG" then v = "INFO"
elseif v == "NOTICE" then v = "INFO"
elseif v == "PANIC" then v = "CRITICAL"
elseif v == "STATEMENT" then v = "DEBUG"
elseif v == "WARNING" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
