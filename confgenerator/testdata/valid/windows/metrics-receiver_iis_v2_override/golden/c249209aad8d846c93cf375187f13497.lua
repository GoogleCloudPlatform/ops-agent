
function process(tag, timestamp, record)
    io.write(tag, ": [3] end message=", record["Message"], "\n");
    return 0, timestamp, record
end