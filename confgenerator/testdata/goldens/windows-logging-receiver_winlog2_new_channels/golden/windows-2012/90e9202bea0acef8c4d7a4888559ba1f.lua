
function process(tag, timestamp, record)
    severityKey = 'logging.googleapis.com/severity'
    if record['Level'] == 1 then
        record[severityKey] = 'CRITICAL'
    elseif record['Level'] == 2 then
        record[severityKey] = 'ERROR'
    elseif record['Level'] == 3 then
        record[severityKey] = 'WARNING'
    elseif record['Level'] == 4 then
        record[severityKey] = 'INFO'
    elseif record['Level'] == 5 then
        record[severityKey] = 'NOTICE'
    end
    return 2, timestamp, record
end
