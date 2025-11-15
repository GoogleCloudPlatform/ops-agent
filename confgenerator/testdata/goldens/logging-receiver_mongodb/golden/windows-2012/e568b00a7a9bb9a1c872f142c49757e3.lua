
function process(tag, timestamp, record)
local __field_0 = (function()
return record["c"]
end)();
(function(value)
record["c"] = value
end)(nil);
local v = __field_0;
(function(value)
record["component"] = value
end)(v)
return 2, timestamp, record
end
