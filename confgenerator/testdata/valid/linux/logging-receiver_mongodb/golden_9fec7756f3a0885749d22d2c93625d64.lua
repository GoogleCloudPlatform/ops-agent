
function process(tag, timestamp, record)
local __field_1 = (function()
return record["severity"]
end)();
local v = "agent.googleapis.com/mongodb";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "D" then v = "DEBUG"
elseif v == "D1" then v = "DEBUG"
elseif v == "D2" then v = "DEBUG"
elseif v == "D3" then v = "DEBUG"
elseif v == "D4" then v = "DEBUG"
elseif v == "D5" then v = "DEBUG"
elseif v == "E" then v = "ERROR"
elseif v == "F" then v = "FATAL"
elseif v == "I" then v = "INFO"
elseif v == "W" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
