# Use a minimal image as a parent image
FROM golang

# Set the working directory to /npm
WORKDIR /npm

# Copy the current directory contents into the container at /npm
ADD informer.go npm /npm/

# Make port 80 available to the world outside this container
#EXPOSE 80

# Define environment variable
ENV NAME npm

# Run the npm command by default when the container starts.
ENTRYPOINT /npm/npm