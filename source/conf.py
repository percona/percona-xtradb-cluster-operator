# Configuration file for the Sphinx documentation builder.
#
# This file only contains a selection of the most common options. For a full
# list see the documentation:
# http://www.sphinx-doc.org/en/master/config

# -- Path setup --------------------------------------------------------------

# If extensions (or modules to document with autodoc) are in another directory,
# add these directories to sys.path here. If the directory is relative to the
# documentation root, use os.path.abspath to make it absolute, like shown here.
#
import sys, os
# import sys
# test import

# -- General configuration ---------------------------------------------------

# Add any Sphinx extension module names here, as strings. They can be
# extensions coming with Sphinx (named 'sphinx.ext.*') or your custom
# ones.
sys.path.append(os.path.abspath('ext'))

extensions=['sphinx.ext.intersphinx', 'sphinx.ext.todo',
            'sphinx.ext.coverage', 'sphinx.ext.ifconfig','sphinx.ext.extlinks', 'sphinx-prompt', 'fulltoc', 'psdom', ]
# Add any paths that contain templates here, relative to this directory.
templates_path = ['_templates']

# the suffix of source filenames.
source_suffix = '.rst'

# the master toctree document
master_doc = 'index'

# General information about the project.
project = u'Percona Distribution for MySQL Operator based on Percona XtraDB Cluster'
copyright = u'Percona LLC and/or its affiliates 2009 - 2022'

# the short X.Y version
version = '1.11.0'
# the full version including alpha/beta/rc tags.
release = '1.11.0'
# the PXC 5.7 and PXC 8.0 recommended versions to be used in docs
pxc57recommended = '5.7.36-31.55'
pxc80recommended = '8.0.27-18.1'
pmm2recommended = '2.28.0'
gkerecommended = '1.23'

# List of patterns, relative to source directory, that match files and
# directories to ignore when looking for source files.
# This pattern also affects html_static_path and html_extra_path.
exclude_patterns = ['*.txt']

# the reST default role (used for this markup: 'text') to use for all documents.
# default_role = none

###primary_domain = 'psdom'

# the name of the Pygments (syntax highlighting) style to use.
pygments_style = 'sphinx'

rst_prolog = '''
.. |check|  replace:: ``|[[---CHECK---]]|``

.. |xtrabackup|  replace:: :program:`xtrabackup`

.. |innobackupex|  replace:: :program:`innobackupex`

.. |XtraDB|  replace:: *XtraDB*

.. |Percona XtraDB Cluster|  replace:: *Percona XtraDB Cluster*

.. |InnoDB|  replace:: *InnoDB*

.. |MongoDB|  replace:: *MongoDB*

.. |MyISAM|  replace:: *MyISAM*

.. |XtraBackup|  replace:: *XtraBackup*

.. |Percona XtraBackup|  replace:: *Percona XtraBackup*

.. |Percona Server|  replace:: *Percona Server*

.. |Percona|  replace:: *Percona*

.. |pmm|  replace:: *PMM*

.. |pmm-server|  replace:: *PMM Server*

.. |pmm-client|  replace:: *PMM Client*

.. |postgresql|  replace:: *PostgreSQL*

.. |pmm-add-instance| replace:: *PMM Add Instance*

.. |gui.pmm-dropdown| replace:: :guilabel:`PMM Dropdown`

.. |MySQL|  replace:: *MySQL*

.. |sysbench|  replace:: *sysbench*

.. |PXC|  replace:: *Percona XtraDB Cluster*

.. |Drizzle|  replace:: *Drizzle*

.. |tar4ibd|  replace:: :program:`tar4ibd`

.. |tar|  replace:: :program:`tar`

'''

extlinks = {'bug':
('https://bugs.launchpad.net/percona-xtradb-cluster/+bug/%s',
                      '#'),
'jirabug':
('https://jira.percona.com/browse/%s',
                      ''),
'mysqlbug':
('http://bugs.mysql.com/bug.php?id=%s',
                      '#'),
'githubbug':
('https://github.com/codership/galera/issues/%s',
                      '#'),
'wsrepbug':
('https://github.com/codership/mysql-wsrep/issues/%s',
                      '#'),
'cloudjira':
('https://jira.percona.com/browse/CLOUD-%s',
                      'CLOUD-')}
# A list of ignored prefixes for module index sorting.
#modindex_common_prefix = []





# -- Options for HTML output ---------------------------------------------------

# The theme to use for HTML and HTML Help pages.  See the documentation for
# a list of builtin themes.
html_theme = 'percona-theme'
#html_add_permalinks = ""
# Theme options are theme-specific and customize the look and feel of a theme
# further.  For a list of options available for each theme, see the
# documentation.
#html_theme_options = {}

# Add any paths that contain custom themes here, relative to this directory.
html_theme_path = ['.', './percona-theme']

# The name for this set of Sphinx documents.  If None, it defaults to
# "<project> v<release> documentation".
html_title = 'Percona Distribution for MySQL Operator based on Percona XtraDB Cluster - Documentation'

# A shorter title for the navigation bar.  Default is the same as html_title.
html_short_title = 'Percona Distribution for MySQL Operator based on Percona XtraDB Cluster - Documentation'

# The name of an image file (relative to this directory) to place at the top
# of the sidebar.
#html_logo = 'percona-xtrabackup-logo.jpg'
html_logo = ''

# The name of an image file (within the static path) to use as favicon of the
# docs.  This file should be a Windows icon file (.ico) being 16x16 or 32x32
# pixels large.
html_favicon = 'percona_favicon.ico'

# Add any paths that contain custom static files (such as style sheets) here,
# relative to this directory. They are copied after the builtin static files,
# so a file named "default.css" will overwrite the builtin "default.css".
html_static_path = ['_static']

# If not '', a 'Last updated on:' timestamp is inserted at every page bottom,
# using the given strftime format.
#html_last_updated_fmt = '%b %d, %Y'

# If true, SmartyPants will be used to convert quotes and dashes to
# typographically correct entities.
#html_use_smartypants = True

# Custom sidebar templates, maps document names to template names.
#html_sidebars = {}

# Additional templates that should be rendered to pages, maps page names to
# template names.
#html_additional_pages = {}

# If false, no module index is generated.
#html_domain_indices = True

# If false, no index is generated.
#html_use_index = True

# If true, the index is split into individual pages for each letter.
#html_split_index = False

# If true, links to the reST sources are added to the pages.
#html_show_sourcelink = True

# If true, "Created using Sphinx" is shown in the HTML footer. Default is True.
#html_show_sphinx = True

# If true, "(C) Copyright ..." is shown in the HTML footer. Default is True.
#html_show_copyright = True

# If true, an OpenSearch description file will be output, and all pages will
# contain a <link> tag referring to it.  The value of this option must be the
# base URL from which the finished HTML is served.
#html_use_opensearch = ''

# This is the file name suffix for HTML files (e.g. ".xhtml").
#html_file_suffix = None

# Output file base name for HTML help builder.
htmlhelp_basename = 'pxcoperatorpxc'


# -- Options for LaTeX output --------------------------------------------------

# The paper size ('letter' or 'a4').
#latex_paper_size = 'letter'

# The font size ('10pt', '11pt' or '12pt').
#latex_font_size = '10pt'

# Grouping the document tree into LaTeX files. List of tuples
# (source start file, target name, title, author, documentclass [howto/manual]).
latex_documents = [
  ('index', 'percona-kubernetes-operator-for-mysql-pxc.tex', u'Percona Distribution for MySQL Operator based on Percona XtraDB Cluster',
     u'Percona LLC and/or its affiliates 2009-2022', 'manual'),
]

# The name of an image file (relative to this directory) to place at the top of
# the title page.
latex_logo = 'percona-logo.jpg'

# For "manual" documents, if this is true, then toplevel headings are parts,
# not chapters.
#latex_use_parts = False
latex_toplevel_sectioning = 'part'

# If true, show page references after internal links.
#latex_show_pagerefs = False

# If true, show URL addresses after external links.
#latex_show_urls = False

# Additional stuff for the LaTeX preamble.
latex_preamble = '\setcounter{tocdepth}{2}'

# Documents to append as an appendix to all manuals.
#latex_appendices = []

# If false, no module index is generated.
#latex_domain_indices = True

latex_elements = {
  'extraclassoptions': 'openany,oneside'
}

# -- Options for manual page output --------------------------------------------

# One entry per manual page. List of tuples
# (source start file, name, description, authors, manual section).
man_pages = [
    ('index', 'percona-kubernetes-operator-for-mysql-pxc', u'Percona Distribution for MySQL Operator based on Percona XtraDB Cluster',
     [u'Percona LLC and/or its affiliates 2009-2022'], 1)
]

def ultimateReplace(app, docname, source):
    result = source[0]
    for key in app.config.ultimate_replacements:
        result = result.replace(key, app.config.ultimate_replacements[key])
    source[0] = result

ultimate_replacements = {
    "{{{release}}}" : release,
    "{{{apiversion}}}" : release.replace(".", "-", 2),
    "{{{pxc57recommended}}}" : pxc57recommended,
    "{{{pxc80recommended}}}" : pxc80recommended,
    "{{{pmm2recommended}}}" : pmm2recommended,
    "{{{gkerecommended}}}" : gkerecommended
}

def setup(app):
   app.add_config_value('ultimate_replacements', {}, True)
   app.connect('source-read', ultimateReplace)
