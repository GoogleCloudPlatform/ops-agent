
function process(tag, timestamp, record)
local __field_0 = (function()
return record["cs_uri_query"]
end)();
local __field_1 = (function()
return record["http_request_referer"]
end)();
local __field_2 = (function()
return record["user"]
end)();
local omit0 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["cs_uri_query"]
end)());
local omit1 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["http_request_referer"]
end)());
local omit2 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["user"]
end)());
local v = __field_0;
if omit0 then v = nil end;
(function(value)
record["cs_uri_query"] = value
end)(v)
local v = __field_1;
if omit1 then v = nil end;
(function(value)
record["http_request_referer"] = value
end)(v)
local v = __field_2;
if omit2 then v = nil end;
(function(value)
record["user"] = value
end)(v)
return 2, timestamp, record
end
