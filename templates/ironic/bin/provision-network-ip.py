#!/bin/env python3

import os
import psutil
import socket

def set_provision_ip():
    for nic, addrs in psutil.net_if_addrs().items():
        if nic.startswith('eth') or nic == 'lo':
            continue
        for addr in addrs:
            if addr.family == socket.AF_INET:
                print(addr.address)
                return

if __name__ == '__main__':
    set_provision_ip()