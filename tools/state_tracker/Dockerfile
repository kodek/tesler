# iron/go is the alpine image with only ca-certificates added
#FROM iron/go
FROM golang:1.18.3
WORKDIR /app
# Now just add the binary 
COPY ./templates /app/templates/
COPY state_tracker .

ENV TZ America/Los_Angeles

RUN echo $TZ > /etc/timezone && \
    apt-get update && apt-get install -y tzdata && \
    rm /etc/localtime && \
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && \
    dpkg-reconfigure -f noninteractive tzdata && \
    apt-get clean

ENTRYPOINT ["./state_tracker"]
