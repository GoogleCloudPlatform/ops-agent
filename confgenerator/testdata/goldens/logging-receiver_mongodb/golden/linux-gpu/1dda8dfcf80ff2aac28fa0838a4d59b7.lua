
function process(tag, timestamp, record)
local __field_0 = (function()
return record["msg"]
end)();
(function(value)
record["msg"] = value
end)(nil);
local v = __field_0;
(function(value)
record["message"] = value
end)(v)
return 2, timestamp, record
end
