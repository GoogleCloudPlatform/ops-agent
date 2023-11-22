
function process(tag, timestamp, record)
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
