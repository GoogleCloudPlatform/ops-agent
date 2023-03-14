
function process(tag, timestamp, record)
    local v = record["raw_xml"];
    if v == nil then v = "nil" end;
    record["check1"] = v;
    record["check1_tag"] = tag;
    return 2, timestamp, record
end