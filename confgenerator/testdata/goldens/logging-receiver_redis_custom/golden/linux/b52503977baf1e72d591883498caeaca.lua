
function process(tag, timestamp, record)
local __field_0 = (function()
return record["agent.googleapis.com/log_file_path"]
end)();
local __field_1 = (function()
if record["logging.googleapis.com/labels"] == nil
then
return nil
end
return record["logging.googleapis.com/labels"]["compute.googleapis.com/instance_group_manager/name"]
end)();
local __field_2 = (function()
if record["logging.googleapis.com/labels"] == nil
then
return nil
end
return record["logging.googleapis.com/labels"]["compute.googleapis.com/instance_group_manager/zone"]
end)();
local __field_3 = (function()
if record["logging.googleapis.com/labels"] == nil
then
return nil
end
return record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"]
end)();
local __field_4 = (function()
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
if v == nil then v = "test-mig" end;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/instance_group_manager/name"] = value
end)(v)
local v = __field_2;
if v == nil then v = "test-zone" end;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/instance_group_manager/zone"] = value
end)(v)
local v = __field_3;
if v == nil then v = "" end;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"] = value
end)(v)
local v = __field_4;
if v == nil then v = "redis_syslog" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
