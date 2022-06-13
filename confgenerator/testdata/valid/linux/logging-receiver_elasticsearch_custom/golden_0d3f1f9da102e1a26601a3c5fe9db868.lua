
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/elasticsearch_json";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
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
