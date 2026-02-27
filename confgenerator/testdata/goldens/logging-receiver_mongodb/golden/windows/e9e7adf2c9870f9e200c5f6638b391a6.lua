
function process(tag, timestamp, record)
local __field_0 = (function()
return record["ctx"]
end)();
(function(value)
record["ctx"] = value
end)(nil);
local v = __field_0;
(function(value)
record["context"] = value
end)(v)
return 2, timestamp, record
end
