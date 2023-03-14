
function process(tag, timestamp, record)
	record["check3"] = "true";
	record["check3_tag"] = tag;
	return 2, timestamp, record
end