
function process(tag, timestamp, record)
local __field_0 = (function()
return record["fileSort"]
end)();
local __field_1 = (function()
return record["fileSortOnDisk"]
end)();
local __field_2 = (function()
return record["fullJoin"]
end)();
local __field_3 = (function()
return record["fullScan"]
end)();
local __field_4 = (function()
return record["priorityQueue"]
end)();
local __field_5 = (function()
return record["qcHit"]
end)();
local v = __field_0;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["fileSort"] = value
end)(v)
local v = __field_1;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["fileSortOnDisk"] = value
end)(v)
local v = __field_2;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["fullJoin"] = value
end)(v)
local v = __field_3;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["fullScan"] = value
end)(v)
local v = __field_4;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["priorityQueue"] = value
end)(v)
local v = __field_5;

local v2 = (v and v == "Yes")
if v2 ~= fail then v = v2
end
(function(value)
record["qcHit"] = value
end)(v)
local v = "agent.googleapis.com/mysql_slow";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
return 2, timestamp, record
end
