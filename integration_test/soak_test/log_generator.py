"""
Copyright 2023 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

===============================================================================

Log Generator for performance testing.

This component generates logs with fixed size at a given rate. It should be run
as a daemon.

To start the generator:
python3 log_generator.py
or
python3 log_generator.py --log-size-in-bytes=100 --log-rate=100
"""

# Please keep this file in sync with the copy in Piper.

import abc
import argparse
import itertools
import logging
import os
import random
import socket
import string
import sys
import time


logging.basicConfig(
    level=logging.DEBUG, stream=sys.stderr,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger('log_generator')
logger.setLevel(logging.DEBUG)

# Prefix of the log tag.
_TAG_PREFIX = 'performance-benchmarking'
# The default collection of characters that we use to generate a random log.
_DEFAULT_CHARS = string.ascii_letters + string.digits


def _log_error_and_sleep(err, sleep_seconds):
  """Prints the error and sleeps for the specified period.

  Args:
    err: socket.error.
      The error to print.
    sleep_seconds: int.
      The number of seconds to sleep.
  """
  logger.error('Encountered error: %s.', err)
  logger.info('Sleep for %d seconds then try again.', sleep_seconds)
  time.sleep(sleep_seconds)


class LogDistribution(object):
  """Specifies a distribution of logs to be generated."""

  def __init__(self, log_size_in_bytes, log_rate):
    self.log_size_in_bytes = log_size_in_bytes
    self.log_rate = log_rate
    self.log_tag = '{prefix}.size-{size}-rate-{rate}'.format(
        prefix=_TAG_PREFIX,
        size=log_size_in_bytes,
        rate=log_rate)
    # Calculate how much padding is needed to get the log length to
    # the desired number of bytes.
    smallest_log = self._generate_log('')
    self.padding_size = log_size_in_bytes - len(smallest_log)
    if self.padding_size < 0:
      # Impossible to make a log this small. This is because we also put
      # metadata in each log.
      raise ValueError('Smallest possible log has length {}: {}'.format(
          len(smallest_log), smallest_log))
    self.last_delta = 0
    self.last_log = self._generate_log(self._get_padding(0))
    self.start_time = time.time()

  def _get_padding(self, seed):
    """Generate padding of length self.padding_size.

    A log entry size is derived from the custom tag size and the padding size.
    The custom tag size is not static as it contains benchmark, the log rate and
    log size info for easy debugging. So we calculate self.padding_size
    to make the log entry have just the right length in __init__, and here
    we return an entry with the given size.

    Args:
      seed: the seed to pass into the pseudo-random number generator. This
        is tied to the log number so that it cycles every second.
    Returns:
      padding of length self.padding_size, which when put into a log will pad
      its length to self.log_size_in_bytes
    """
    random.seed(a=seed, version=2)
    return ''.join(random.choices(_DEFAULT_CHARS, k=self.padding_size))

  def _get_nth_delta(self, n: int) -> int:
    """Number of seconds between the start_time and the nth log message."""
    # Output log_rate entries every second. The // rounds down to the nearest
    # second.
    return n // self.log_rate

  def get_nth_time(self, n: int) -> float:
    """Absolute time at which the nth log message can be generated."""
    return self.start_time + self._get_nth_delta(n)

  def _generate_log(self, padding: str):
    """Creates a log message with the given additional padding.

    A log message should be a list of 3 elements:

    * A string type log tag. Fluentd requires every log event to be tagged with
      a string tag. All of the filters and output plugins match a specific set
      of tags. The tag can be used later to filter the logs. In the Logs Viewer,
      the second drop-down list is "tag".
    * Seconds since epoch timestamp.
    * A JSON type log record.
    Args:
      padding: Padding to add to the message to reach a given length.
    Returns:
      The full log message.
    """
    return '["{tag}", {timestamp}, {{"log": "{log_record}"}}]'.format(
        tag=self.log_tag, timestamp=int(time.time()), log_record=padding)

  def get_nth_log(self, n: int):
    """Constructs the nth full log message."""
    if self._get_nth_delta(n) != self.last_delta:
      self.last_delta = self._get_nth_delta(n)
      self.last_log = self._generate_log(self._get_padding(n))
    return self.last_log


class LogGenerator(object):
  """Given a log distribution, generates logs matching that distribution."""

  def __init__(self, log_writer, log_distribution):
    self.log_writer = log_writer
    self.log_distribution = log_distribution

  def run(self, count: int):
    """Generate logs.

    The high level idea here is that we sit in a loop waiting until a log entry
    can be printed, and then print it. The benefits of this approach are:
    * No skew overtime as the log times are calculated against the
      start_time.
    * Precise detection of overruns. The code knows exactly how far behind
      schedule it is, since it knows when every log should be printed.
    * Can allow more complex logging patterns like dump logs every
      minute instead of second, ramping up log generation, or periodical log
      throughputs.

    Args:
      count: how many logs to produce. If count <= 0, produce logs forever.
    """
    # last_checked_time is used to optimize away time.time calls.
    last_checked_time = 0
    for n in itertools.count(start=1):
      expected_nth_log_time = self.log_distribution.get_nth_time(n)
      # If last_checked_time >= expected_nth_log_time,
      # there is no need for any waiting logic.
      if expected_nth_log_time > last_checked_time:
        while expected_nth_log_time > time.time():
          # Sleep until approximately 5ms before the next entry should be
          # generated.
          time.sleep(max(expected_nth_log_time - time.time() - 0.05, 0))
        last_checked_time = time.time()
        if last_checked_time - expected_nth_log_time > 1:
          # We only print delays after checking the wall clock
          # and seeing we are more than 1 second behind.
          logger.error(
              'Detected overruns. Log Generator %s seconds behind schedule.',
              last_checked_time - expected_nth_log_time)
      self.log_writer.write_log(self.log_distribution.get_nth_log(n))
      if count == n:
        break


_FILE_OPEN_RETRY_DELAY_IN_SECONDS = 0.5
_FILE_OPEN_RETRIES = 10


class LogWriter(abc.ABC):
  """Writes logs to be picked up by the logging agent."""

  @abc.abstractmethod
  def write_log(self, log: str) -> None:
    """Writes the given log line."""


class FileWriter(LogWriter):
  """Writes logs to a file that will be picked up by the logging agent."""

  def __init__(self, file_location, file_log_limit):
    self.file_location = file_location
    self.file_log_limit = file_log_limit
    self._reset()

  def _reset(self):
    """Resets the number of lines written and opens a new file descriptor."""
    self.written = 0
    # Retries are needed in windows, since moves are not guaranteed to be atomic
    # for more info see the following links:
    # https://stackoverflow.com/questions/8107352/force-overwrite-in-os-rename
    # https://hg.python.org/cpython/file/v3.4.4/Modules/posixmodule.c#l4289
    # https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexa
    for i in range(_FILE_OPEN_RETRIES):
      try:
        self.f = open(self.file_location, 'a')
        return
      except OSError as e:
        if i + 1 == _FILE_OPEN_RETRIES:
          raise
        logging.warning('Retrying opening %s due to %s', self.file_location, e)
        time.sleep(_FILE_OPEN_RETRY_DELAY_IN_SECONDS)

  def write_log(self, log):
    self.f.write(log + '\n')
    self.written += 1
    if self.written >= self.file_log_limit:
      # Rotate the current file. Note that this is not thread-safe.
      self.f.close()
      rotated_path = f'{self.file_location}.old'
      os.replace(self.file_location, rotated_path)
      logger.info('Rotated file to %s.', rotated_path)
      self._reset()


_IN_FORWARD_PLUGIN_HOST = '127.0.0.1'
_IN_FORWARD_PLUGIN_PORT = 24224


class SocketWriter(LogWriter):
  """Socket connection to the in_forward plugin.

  Responsible for establishing a socket connection with the in_forward plugin
  of Logging Agent and sending message via this socket.
  """

  def __init__(self, retry_sleep_seconds):
    """Constructor.

    Args:
      retry_sleep_seconds: int.
        The number of seconds to sleep between each retry when failed to talk to
        the in_forward plugin due to socket errors.
    """
    self._retry_sleep_seconds = retry_sleep_seconds
    # Whether the connection is still healthy. After a failed attempt, we need
    # to reset the connection in order to avoid getting a "Bad File Descriptor"
    # error.
    self._init_connection()
    logger.info('Log generator ready to produce traffic.')

  def _init_connection(self):
    """Initiates a new connection."""
    self._is_connected = False
    self._connection = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    logger.info('Init connection.')

  def _connect(self):
    """Connects to the socket.

    Raises:
      socket.error: Failed to connect to the in_forward plugin.
    """
    logger.info('Connecting to in_forward plugin at %s:%d.',
                _IN_FORWARD_PLUGIN_HOST, _IN_FORWARD_PLUGIN_PORT)
    try:
      self._connection.connect((
          _IN_FORWARD_PLUGIN_HOST, _IN_FORWARD_PLUGIN_PORT))
      self._is_connected = True
      logger.info('Successfully connected.')
    except socket.error as err:
      self._print_error_and_reset_connection('Failed to connect', err)
      raise

  def _send_message(self, log_message):
    """Sends the log message.

    Args:
      log_message: str.
        The log message to send.
    Raises:
      socket.error: Failed to send message to the in_forward plugin.
    """
    try:
      self._connection.sendall(str.encode(log_message, 'utf-8'))
    except socket.error as err:
      self._print_error_and_reset_connection('Failed to send message', err)
      raise

  def _print_error_and_reset_connection(self, message, err):
    """Prints the error, closes the connection, and initiates a new connection.

    Args:
      message: str.
        The error message content.
      err: socket.error.
        The error we get when talking to the in_forward plugin.
    """
    logger.error('%s due to error:\n%s. Please make sure the Logging Agent'
                 ' is running with the in_forward plugin properly set up at'
                 ' %s:%d.', message, err, _IN_FORWARD_PLUGIN_HOST,
                 _IN_FORWARD_PLUGIN_PORT)
    logger.info('Closed connection.')
    self._connection.close()
    self._init_connection()

  def write_log(self, log):
    """Sends the log message. Unlimited retries on failures."""
    while True:
      try:
        if not self._is_connected:
          self._connect()
        self._send_message(log)
        return
      except socket.error as err:
        _log_error_and_sleep(err, self._retry_sleep_seconds)


# Main function.
parser = argparse.ArgumentParser(
    description='Flags to initiate Log Generator.')
parser.add_argument(
    '--log-size-in-bytes', type=int, default=10,
    help='The size of each log entry in bytes for fixed-entry logs.')
parser.add_argument(
    '--log-rate', type=int, default=10,
    help='The number of expected log entries per second for fixed-rate logs.')
parser.add_argument(
    '--retry-sleep-seconds', type=int, default=1,
    help='The number of seconds to sleep between each retry when failed to'
         ' talk to the in_forward plugin due to socket errors.')
parser.add_argument(
    '--log-write-type', type=str, default='socket',
    choices=['socket', 'file'],
    help='The method by which logs will written and picked up by the'
         ' logging agent. "socket" corresponds to forward, "file" to tail.')

parser.add_argument(
    '--file-path', type=str, default='tail_log',
    help='The file to which logs will be written to.')
parser.add_argument(
    '--file-log-limit', type=int, default=50_000_000,
    help='The maximum number of log lines written to the file before rotating'
         ' it.')

parser.add_argument(
    '--count', type=int, default=-1,
    help='How many logs to send. If not positive, send indefinitely')


def main():
  args = parser.parse_args()
  logger.info('Parsed args: %s', args)
  type_to_writers = {
      'file': lambda: FileWriter(args.file_path, args.file_log_limit),
      'socket': lambda: SocketWriter(args.retry_sleep_seconds),
  }
  log_writer = type_to_writers[args.log_write_type]()
  log_distribution = LogDistribution(args.log_size_in_bytes, args.log_rate)
  LogGenerator(log_writer, log_distribution).run(args.count)


if __name__ == '__main__':
  main()
