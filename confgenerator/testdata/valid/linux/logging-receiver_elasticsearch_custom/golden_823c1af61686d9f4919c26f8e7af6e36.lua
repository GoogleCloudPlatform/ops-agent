
function process(tag, timestamp, record)
local v = "elasticsearch_gc";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["logging.googleapis.com/instrumentation_source"] = value
end)(v)
local v = "elasticsearch_gc_custom";
(function(value)
record["logging.googleapis.com/logName"] = value
end)(v)
