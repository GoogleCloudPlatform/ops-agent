
function process(tag, timestamp, record)
local __field_0 = (function()
if record["logging.googleapis.com/labels"] == nil
then
return nil
end
return record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"]
end)();
local __field_1 = (function()
return record["logging.googleapis.com/logName"]
end)();
local v = __field_0;
if v == nil then v = "" end;
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/resource_name"] = value
end)(v)
local v = __field_1;
if v == nil then v = "fluent_forward.collision" end;
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
return 2, timestamp, record
end
