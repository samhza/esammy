import yt_dlp
from yt_dlp import DownloadError
from yt_dlp.utils import DownloadCancelled
import json
import os
import tempfile
import sys

class MyLogger:
  def debug(self, msg):
      if not msg.startswith('[download] '):
          pass
      else:
          self.error(msg)

  def info(self, msg):
      pass

  def warning(self, msg):
      pass

  def error(self, msg):
      print(msg, file=sys.stderr)

url = sys.argv[1]

def filter(info, *, incomplete):
    is_live = info.get('is_live')
    was_live = info.get('was_live')
    if is_live or was_live:
        return 'This is/was a livestream'
    duration = info.get('duration')
    if duration and duration > 60 * 10:
        return 'This video is too long'

filename = ""
with tempfile.NamedTemporaryFile(delete=False) as f:
    try:
        f.close()
        filename = f.name
    except Exception as e:
        os.remove(f.name)
        raise e
ydl_opts = {
    'match_filter': filter,
    'outtmpl': {'default': filename+'.%(ext)s'},
    'noprogress': True,
    'forcefilename': True,
    'logger': MyLogger()
}

with yt_dlp.YoutubeDL(ydl_opts) as ydl:
    try:
        error_code = ydl.download([url])
    except:
        exit(1)
    finally:
        os.remove(filename)
    exit(error_code)
