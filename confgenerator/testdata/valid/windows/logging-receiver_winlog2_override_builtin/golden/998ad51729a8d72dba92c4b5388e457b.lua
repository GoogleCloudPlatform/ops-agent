
function process(tag, timestamp, record)
    local v = record["raw_xml"];
    if v == nil then v = "nil" end;
    io.write(tag, ": [1] xml=", v, "\n");
    return 0, timestamp, record
end