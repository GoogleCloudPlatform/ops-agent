
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/tomcat_system";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "FINE" then v = "DEBUG"
elseif v == "FINER" then v = "DEBUG"
elseif v == "FINEST" then v = "DEBUG"
elseif v == "INFO" then v = "INFO"
elseif v == "SEVERE" then v = "CRITICAL"
elseif v == "WARNING" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
