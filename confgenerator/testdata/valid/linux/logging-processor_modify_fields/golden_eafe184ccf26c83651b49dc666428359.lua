
function process(tag, timestamp, record)
local __field_0 = (function()
return record["source"]
end)();
local __field_1 = (function()
return record["default"]
end)();
local __field_2 = (function()
return record["float"]
end)();
local __field_4 = (function()
return record["integer"]
end)();
local __field_5 = (function()
return record["move_source"]
end)();
local __field_6 = (function()
return record["unnested"]
end)();
local __field_7 = (function()
return record["omitted"]
end)();
local __field_8 = (function()
return record["level"]
end)();
local omit7 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-")) end)((function()
return record["omitted"]
end)());
(function(value)
record["move_source"] = value
end)(nil);
local v = __field_0;
(function(value)
record["copied"] = value
end)(v)
local v = __field_1;
if v == nil then v = "this field was missing" end;
(function(value)
record["default"] = value
end)(v)
local v = __field_2;

local v2 = tonumber(v)
if v2 ~= fail then v = v2
end
(function(value)
record["float"] = value
end)(v)
local v = "world";
(function(value)
record["hello"] = value
end)(v)
local v = __field_4;

local v2 = math.floor(tonumber(v))
if v2 ~= fail then v = v2
end
(function(value)
record["integer"] = value
end)(v)
local v = __field_5;
(function(value)
record["moved"] = value
end)(v)
local v = __field_6;
(function(value)
if record["nested"] == nil
then
record["nested"] = {}
end
if record["nested"]["structure"] == nil
then
record["nested"]["structure"] = {}
end
record["nested"]["structure"]["field"] = value
end)(v)
local v = __field_7;
if omit7 then v = nil end;
(function(value)
record["omitted"] = value
end)(v)
local v = __field_8;
if v == "CAUTION" then v = "WARNING"
elseif v == "I" then v = "INFO"
elseif v == "W" then v = "WARNING"
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
return 2, timestamp, record
end
