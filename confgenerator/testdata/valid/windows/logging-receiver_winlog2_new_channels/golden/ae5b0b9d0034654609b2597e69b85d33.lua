
function process(tag, timestamp, record)
    record["check0"] = record["Message"];
    record["check0_tag"] = tag;
    return 2, timestamp, record
end