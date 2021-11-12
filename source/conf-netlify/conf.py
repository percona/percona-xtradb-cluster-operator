#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import sys, os

sys.path.append(os.path.abspath("../"))

#sys.path.append(os.path.abspath('ext'))
#import sphinx_rtd_theme

from conf import *


#html_theme = "sphinx_rtd_theme"

html_sidebars['**']=['globaltoc.html', 'searchbox.html', 'localtoc.html', 'logo-text.html']
html_theme = 'sphinx_material'
html_theme_options = {
    'base_url': 'http://bashtage.github.io/sphinx-material/',
    'repo_url': 'https://github.com/percona/percona-xtradb-cluster-operator/tree/pxc-docs', #Creates the GitHub repo label to edit the docs
    'repo_name': 'percona/percona-xtradb-cluster-operator',
    'color_accent': 'grey',  
    'color_primary': 'orange',  #Theme colors to match existing palette
    'globaltoc_collapse': True,
    'version_dropdown': False, #Controls the version dropdown
    'version_dropdown_text': 'Versions',
    'version_info': {
        "3.6": "https://docs.percona.com/percona-server-for-mongodb/3.6/", # URL to the existing versions
        "4.0": "https://docs.percona.com/percona-server-for-mongodb/4.0/",
        "4.2": "https://docs.percona.com/percona-server-for-mongodb/4.2/",
        "4.4": "https://docs.percona.com/percona-server-for-mongodb/4.4/",
        "5.0": "https://docs.percona.com/percona-server-for-mongodb/5.0/",
        "Latest": "https://docs.percona.com/percona-server-for-mongodb/4.4/"
    },
}
html_logo = '../_static/images/percona-logo.svg' #Path to docs logo relative to this config
html_favicon = '../_static/images/percona_favicon.ico'  #Path to favicon relative to this config

