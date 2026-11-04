"""Verify a bucket-scoped SeaweedFS S3 identity against an admin identity.

The scoped identity must be able to put, get, list, tag and delete objects
in each of its buckets, and must be refused bucket creation, bucket
deletion, and any access to a bucket outside its scope (a throwaway one the
admin creates and removes). Anonymous requests must be refused.

Credentials come from the environment, normally injected by `op run`;
nothing secret is printed.

env: S3_ENDPOINT, SCOPED_BUCKETS (comma-separated),
     ADMIN_AK, ADMIN_SK, SCOPED_AK, SCOPED_SK

    uv run --no-project --with boto3 python verify.py
"""
import os, sys, uuid, urllib.request, urllib.error
import boto3
from botocore.config import Config
from botocore.exceptions import ClientError
ep = os.environ["S3_ENDPOINT"]
cfg = Config(s3={"addressing_style": "path"}, signature_version="s3v4", retries={"max_attempts": 1})
def client(ak, sk): return boto3.client("s3", endpoint_url=ep, region_name="us-east-1", aws_access_key_id=ak, aws_secret_access_key=sk, config=cfg)
admin = client(os.environ["ADMIN_AK"], os.environ["ADMIN_SK"])
cad = client(os.environ["SCOPED_AK"], os.environ["SCOPED_SK"])
buckets = os.environ["SCOPED_BUCKETS"].split(",")
ok = True
def check(name, fn, expect_ok=True):
    global ok
    try:
        fn(); res = True; detail = ""
    except ClientError as e:
        res = False; detail = e.response["Error"]["Code"]
    good = res == expect_ok
    ok &= good
    print(f"{'PASS' if good else 'FAIL'}  {name}: {'allowed' if res else 'denied ' + detail}")
other = "probe-other-" + uuid.uuid4().hex[:6]
check("admin creates bucket " + other, lambda: admin.create_bucket(Bucket=other))
for b in buckets:
    k = f"verify/{uuid.uuid4().hex}.txt"
    check(f"[{b}] put", lambda: cad.put_object(Bucket=b, Key=k, Body=b"hello"))
    check(f"[{b}] get", lambda: (_ for _ in ()).throw(AssertionError()) if cad.get_object(Bucket=b, Key=k)["Body"].read() != b"hello" else None)
    check(f"[{b}] list", lambda: None if any(o["Key"] == k for o in cad.list_objects_v2(Bucket=b, Prefix="verify/").get("Contents", [])) else (_ for _ in ()).throw(ClientError({"Error": {"Code": "NotListed"}}, "list")))
    check(f"[{b}] put tagging", lambda: cad.put_object_tagging(Bucket=b, Key=k, Tagging={"TagSet": [{"Key": "t", "Value": "1"}]}))
    check(f"[{b}] get tagging", lambda: cad.get_object_tagging(Bucket=b, Key=k))
    check(f"[{b}] delete", lambda: cad.delete_object(Bucket=b, Key=k))
check("scoped creates bucket", lambda: cad.create_bucket(Bucket="probe-new-" + uuid.uuid4().hex[:6]), expect_ok=False)
check(f"scoped deletes bucket {buckets[-1]}", lambda: cad.delete_bucket(Bucket=buckets[-1]), expect_ok=False)
check(f"scoped puts into {other}", lambda: cad.put_object(Bucket=other, Key="x", Body=b"x"), expect_ok=False)
check(f"scoped lists {other}", lambda: cad.list_objects_v2(Bucket=other), expect_ok=False)
admin.put_object(Bucket=other, Key="x", Body=b"x")
check(f"scoped gets from {other}", lambda: cad.get_object(Bucket=other, Key="x"), expect_ok=False)
try:
    names = [b["Name"] for b in cad.list_buckets()["Buckets"]]
    print("INFO  scoped ListBuckets sees:", names)
except ClientError as e:
    print("INFO  scoped ListBuckets denied:", e.response["Error"]["Code"])
try:
    urllib.request.urlopen(ep.rstrip("/") + "/" + buckets[0] + "?list-type=2", timeout=10); print("FAIL  anonymous list allowed"); ok = False
except urllib.error.HTTPError as e:
    print(f"PASS  anonymous list: denied HTTP {e.code}")
admin.delete_object(Bucket=other, Key="x"); admin.delete_bucket(Bucket=other)
print("ALL PASS" if ok else "SOME FAILED"); sys.exit(0 if ok else 1)
