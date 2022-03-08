
function process(tag, timestamp, record)
local match = ((string.lower((function()
return record["a`~!@#$%^&*()-_=+\\|]}[{<.>/?;:b"]
end)()) == string.lower("foo")))

  if match then
    return -1, 0, 0
  end
  return 0, 0, 0
end