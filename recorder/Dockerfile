# iron/go is the alpine image with only ca-certificates added
#FROM iron/go
FROM golang:1.10.3
WORKDIR /app
# Now just add the binary 
COPY ./templates /app/templates/
COPY recorder_main .
COPY start_tesler.sh .

RUN chmod +x ./start_tesler.sh

ENV TZ America/Los_Angeles

ARG travis_commit
ENV TRAVIS_COMMIT=$travis_commit

RUN echo $TZ > /etc/timezone && \
    apt-get update && apt-get install -y tzdata && \
    rm /etc/localtime && \
    ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && \
    dpkg-reconfigure -f noninteractive tzdata && \
    apt-get clean

ENTRYPOINT ["./start_tesler.sh"]
