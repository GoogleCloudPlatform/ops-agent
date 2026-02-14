
function process(tag, timestamp, record)
local __field_0 = (function()
if record["t"] == nil
then
return nil
end
return record["t"]["$date"]
end)();
(function(value)
if record["t"] == nil
then
record["t"] = {}
end
record["t"]["$date"] = value
end)(nil);
local v = __field_0;
(function(value)
record["time"] = value
end)(v)
return 2, timestamp, record
end
