
function process(tag, timestamp, record)
local v = "apache_access";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = "apache_default_access";
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
