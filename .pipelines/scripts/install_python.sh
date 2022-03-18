sudo apt-get -y install libbz2-dev
wget https://www.python.org/ftp/python/3.9.0/Python-3.9.0.tar.xz
tar -xf Python-3.9.0.tar.xz
cd Python-3.9.0
./configure
sudo make altinstall
python3.9 --version
python3.9 -m pip install requests
python3.9 -m pip install pytz
python3.9 -m pip install azure-storage-blob
python3.9 -m pip install azure-storage-queue
python3.9 -m pip install azure-kusto-ingest
python3.9 -m pip install azure-kusto-data
python3.9 -m pip install pandas
python3.9 -m pip install azure-cosmosdb-table
python3.9 -m pip install azure-storage-common
python3.9 -m pip install tabulate
