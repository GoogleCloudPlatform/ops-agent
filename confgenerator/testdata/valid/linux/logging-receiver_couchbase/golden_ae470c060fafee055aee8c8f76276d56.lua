
function process(tag, timestamp, record)
local __field_0 = (function()
return record["agent.googleapis.com/log_file_path"]
end)();
local __field_1 = (function()
if record["logging.googleapis.com/labels"] == nil
then
return nil
end
return record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"]
end)();
local __field_2 = (function()
return record["logging.googleapis.com/logName"]
end)();
(function(value)
record["agent.googleapis.com/log_file_path"] = value
end)(nil);
local v = __field_0;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["agent.googleapis.com/log_file_path"] = value
end)(v)
local v = __field_1;
if v == nil then v = "" end;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"] = value
end)(v)
local v = __field_2;
if v == nil then v = "couchbase_goxdcr" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
