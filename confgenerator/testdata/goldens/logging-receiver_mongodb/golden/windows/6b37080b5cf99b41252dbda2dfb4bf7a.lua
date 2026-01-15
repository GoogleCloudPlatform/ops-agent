
function process(tag, timestamp, record)
local __field_0 = (function()
if record["attr"] == nil
then
return nil
end
return record["attr"]["message"]
end)();
(function(value)
if record["attr"] == nil
then
record["attr"] = {}
end
record["attr"]["message"] = value
end)(nil);
local v = __field_0;
(function(value)
record["temp_attributes_message"] = value
end)(v)
return 2, timestamp, record
end
