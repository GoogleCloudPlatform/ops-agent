
function process(tag, timestamp, record)
local __field_1 = (function()
return record["level"]
end)();
local v = "agent.googleapis.com/mysql_error";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_1;
if v == "ERROR" then v = "ERROR"
elseif v == "Error" then v = "ERROR"
elseif v == "NOTE" then v = "NOTICE"
elseif v == "Note" then v = "NOTICE"
elseif v == "SYSTEM" then v = "INFO"
elseif v == "System" then v = "INFO"
elseif v == "WARNING" then v = "WARNING"
elseif v == "Warning" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
