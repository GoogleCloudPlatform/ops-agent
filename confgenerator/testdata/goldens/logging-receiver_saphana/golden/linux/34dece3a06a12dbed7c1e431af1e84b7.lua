
function process(tag, timestamp, record)
local __field_0 = (function()
return record["connection_id"]
end)();
local __field_1 = (function()
return record["transaction_id"]
end)();
local __field_2 = (function()
return record["update_transaction_id"]
end)();
local __field_4 = (function()
return record["severity_flag"]
end)();
local __field_5 = (function()
return record["source_file"]
end)();
local __field_6 = (function()
return record["source_line"]
end)();
local omit0 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-1")) end)((function()
return record["connection_id"]
end)());
local omit1 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-1")) end)((function()
return record["transaction_id"]
end)());
local omit2 = (function(v) if v == nil then return false end return (string.lower(tostring(v)) == string.lower("-1")) end)((function()
return record["update_transaction_id"]
end)());
(function(value)
record["source_file"] = value
end)(nil);
(function(value)
record["source_line"] = value
end)(nil);
local v = __field_0;
if omit0 then v = nil end;
(function(value)
record["connection_id"] = value
end)(v)
local v = __field_1;
if omit1 then v = nil end;
(function(value)
record["transaction_id"] = value
end)(v)
local v = __field_2;
if omit2 then v = nil end;
(function(value)
record["update_transaction_id"] = value
end)(v)
local v = "agent.googleapis.com/saphana";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = __field_4;
if v == "d" then v = "DEBUG"
elseif v == "e" then v = "ERROR"
elseif v == "f" then v = "ALERT"
elseif v == "i" then v = "INFO"
elseif v == "w" then v = "WARNING"
else v = nil
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
local v = __field_5;
(function(value)
if record["logging.googleapis.com/sourceLocation"] == nil
then
record["logging.googleapis.com/sourceLocation"] = {}
end
record["logging.googleapis.com/sourceLocation"]["file"] = value
end)(v)
local v = __field_6;
(function(value)
if record["logging.googleapis.com/sourceLocation"] == nil
then
record["logging.googleapis.com/sourceLocation"] = {}
end
record["logging.googleapis.com/sourceLocation"]["line"] = value
end)(v)
return 2, timestamp, record
end
