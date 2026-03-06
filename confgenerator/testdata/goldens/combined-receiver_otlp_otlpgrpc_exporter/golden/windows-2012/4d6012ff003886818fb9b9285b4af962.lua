
function process(tag, timestamp, record)
local __field_0 = (function()
return record["severity"]
end)();
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
return 2, timestamp, record
end
