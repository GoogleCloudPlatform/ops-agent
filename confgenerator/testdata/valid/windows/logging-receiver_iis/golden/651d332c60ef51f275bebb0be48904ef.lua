
function process(tag, timestamp, record)
    record["check2"] = "true";
    record["check2_tag"] = tag;
    return 2, timestamp, record
end