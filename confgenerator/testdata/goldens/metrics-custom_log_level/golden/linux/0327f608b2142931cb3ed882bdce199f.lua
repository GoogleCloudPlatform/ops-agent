
function process(tag, timestamp, record)
local __field_0 = (function()
return record["severity"]
end)();
local __field_1 = (function()
if record["sourceLocation"] == nil
then
return nil
end
return record["sourceLocation"]["file"]
end)();
local __field_2 = (function()
if record["sourceLocation"] == nil
then
return nil
end
return record["sourceLocation"]["function"]
end)();
local __field_3 = (function()
if record["sourceLocation"] == nil
then
return nil
end
return record["sourceLocation"]["line"]
end)();
(function(value)
if record["sourceLocation"] == nil
then
record["sourceLocation"] = {}
end
record["sourceLocation"]["file"] = value
end)(nil);
(function(value)
if record["sourceLocation"] == nil
then
record["sourceLocation"] = {}
end
record["sourceLocation"]["function"] = value
end)(nil);
(function(value)
if record["sourceLocation"] == nil
then
record["sourceLocation"] = {}
end
record["sourceLocation"]["line"] = value
end)(nil);
(function(value)
record["severity"] = value
end)(nil);
local v = __field_0;
if v == "debug" then v = "DEBUG"
elseif v == "error" then v = "ERROR"
elseif v == "info" then v = "INFO"
elseif v == "warn" then v = "WARNING"
end
(function(value)
record["logging.googleapis.com/severity"] = value
end)(v)
local v = __field_1;
(function(value)
if record["logging.googleapis.com/sourceLocation"] == nil
then
record["logging.googleapis.com/sourceLocation"] = {}
end
record["logging.googleapis.com/sourceLocation"]["file"] = value
end)(v)
local v = __field_2;
(function(value)
if record["logging.googleapis.com/sourceLocation"] == nil
then
record["logging.googleapis.com/sourceLocation"] = {}
end
record["logging.googleapis.com/sourceLocation"]["function"] = value
end)(v)
local v = __field_3;
(function(value)
if record["logging.googleapis.com/sourceLocation"] == nil
then
record["logging.googleapis.com/sourceLocation"] = {}
end
record["logging.googleapis.com/sourceLocation"]["line"] = value
end)(v)
return 2, timestamp, record
end
