
function process(tag, timestamp, record)
local __field_0 = (function()
return record["roleChar"]
end)();
local __field_2 = (function()
return record["level"]
end)();
local v = __field_0;
if v == "C" then v = "RDB/AOF_writing_child"
elseif v == "M" then v = "master"
elseif v == "S" then v = "slave"
elseif v == "X" then v = "sentinel"
else v = nil
end
(function(value)
record["role"] = value
end)(v)
local v = "agent.googleapis.com/redis";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_2;
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
