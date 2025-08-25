
function process(tag, timestamp, record)
local __field_0 = (function()
return record["attr"]
end)();
(function(value)
record["attr"] = value
end)(nil);
local v = __field_0;
(function(value)
record["attributes"] = value
end)(v)
return 2, timestamp, record
end
