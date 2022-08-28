
function process(tag, timestamp, record)
local __field_0 = (function()
return record["password"]
end)();
local omit0 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) ~= string.lower("")) end)((function()
return record["password"]
end)());
local v = __field_0;
if omit0 then v = nil end;
(function(value)
record["password"] = value
end)(v)
return 2, timestamp, record
end
