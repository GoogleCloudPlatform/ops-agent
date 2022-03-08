
function process(tag, timestamp, record)
local match = ((record["__match_0_0"] != nil) or (record["__match_0_1"] != nil))

for k, v in pairs(record) do
  if string.match(k, "^__match_.+") then
    record[k] = nil
  end
end

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end