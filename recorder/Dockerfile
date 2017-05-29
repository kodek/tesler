# iron/go is the alpine image with only ca-certificates added
#FROM iron/go
FROM golang:1.8
WORKDIR /app
# Now just add the binary 
COPY ./templates /app/templates/
COPY recorder_main .
ENTRYPOINT ["./recorder_main"]
