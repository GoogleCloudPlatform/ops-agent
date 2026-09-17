
local function trim_newline(s)
    -- Check for a Windows-style carriage return and newline (\r\n)
    if string.sub(s, -2) == "\r\n" then
        return string.sub(s, 1, -3)
    -- Check for a Unix/Linux-style newline (\n)
    elseif string.sub(s, -1) == "\n" then
        return string.sub(s, 1, -2)
    end
    -- If no trailing newline is found, return the original string
    return s
end
function strip_newline(tag, timestamp, record)
  record["message"] = trim_newline(record["message"])
  return 2, timestamp, record
end
