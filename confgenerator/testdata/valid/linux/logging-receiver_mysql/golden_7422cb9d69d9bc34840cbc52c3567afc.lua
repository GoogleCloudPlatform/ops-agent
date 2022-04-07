
function process(tag, timestamp, record)
local __field_0 = (function()
return record["agent.googleapis.com/log_file_path"]
end)();
local __field_1 = (function()
return record["logging.googleapis.com/logName"]
end)();
(function(value)
record["agent.googleapis.com/log_file_path"] = value
end)(nil)
local v = __field_0;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/log_file_path"] = value
end)(v)
local v = "mysql_slow";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/receiver_type"] = value
end)(v)
local v = __field_1;
if v == nil then v = "mysql_slow" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
