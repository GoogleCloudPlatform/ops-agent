
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/redis";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "#" then v = "WARNING"
elseif v == "*" then v = "NOTICE"
elseif v == "-" then v = "INFO"
elseif v == "." then v = "DEBUG"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
