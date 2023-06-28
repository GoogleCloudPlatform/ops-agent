
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/cassandra_gc";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "debug" then v = "DEBUG"
elseif v == "develop" then v = "TRACE"
elseif v == "error" then v = "ERROR"
elseif v == "info" then v = "INFO"
elseif v == "trace" then v = "TRACE"
elseif v == "warning" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
