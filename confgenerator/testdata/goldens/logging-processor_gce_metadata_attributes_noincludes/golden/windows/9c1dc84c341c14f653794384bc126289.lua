
function process(tag, timestamp, record)
local v = "${foo:bar}";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/attributes/test-escape"] = value
end)(v)
local v = "test-value";
(function(value)
if record["logging.googleapis.com/labels"] == nil
then
record["logging.googleapis.com/labels"] = {}
end
record["logging.googleapis.com/labels"]["compute.googleapis.com/attributes/test-key"] = value
end)(v)
return 2, timestamp, record
end
