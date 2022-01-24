# Makefile for Sphinx documentation
#

# You can set these variables from the command line.
SPHINXOPTS    =
SPHINXBUILD   = sphinx-build
PAPER         =
BUILDDIR      = build

# Internal variables.
PAPEROPT_a4     = -D latex_paper_size=a4
PAPEROPT_letter = -D latex_paper_size=letter
ALLSPHINXOPTS   = -d $(BUILDDIR)/doctrees $(PAPEROPT_$(PAPER)) $(SPHINXOPTS) source

.PHONY: help clean html dirhtml singlehtml pickle json htmlhelp qthelp devhelp epub latex latexpdf text man changes linkcheck doctest

help:
	@echo "Please use \`make <target>' where <target> is one of"
	@echo "  html       to make standalone HTML files"
	@echo "  offhtml    to make standalone HTML files without fetching fresh percona-them files"
	@echo "  dirhtml    to make HTML files named index.html in directories"
	@echo "  singlehtml to make a single large HTML file"
	@echo "  pickle     to make pickle files"
	@echo "  json       to make JSON files"
	@echo "  htmlhelp   to make HTML files and a HTML help project"
	@echo "  qthelp     to make HTML files and a qthelp project"
	@echo "  devhelp    to make HTML files and a Devhelp project"
	@echo "  epub       to make an epub"
	@echo "  latex      to make LaTeX files, you can set PAPER=a4 or PAPER=letter"
	@echo "  latexpdf   to make LaTeX files and run them through pdflatex"
	@echo "  text       to make text files"
	@echo "  man        to make manual pages"
	@echo "  changes    to make an overview of all changed/added/deprecated items"
	@echo "  linkcheck  to check all external links for integrity"
	@echo "  doctest    to run all doctests embedded in the documentation (if enabled)"

clean:
	-rm -rf $(BUILDDIR)/*

html:
	@echo "Downloading percona-theme ..."
	@wget -O percona-theme.tar.gz https://www.percona.com/docs/theme-1-4/percona-operator-for-pxc/1.0
	@echo "Extracting theme."
	@tar -mzxf percona-theme.tar.gz
	@rm -rf source/percona-theme
	@mv percona-theme-1-4 source/percona-theme
	@rm percona-theme.tar.gz
	#@sed -i 's/{{ toc }}/{{ toctree\(false\) }}/' source/percona-theme/localtoc.html
	@sed -i 's/{{ toc }}/{{ toctree\(false\) }}\n<style>\n\.select-wrapper {\n    display: inline-flex;\n    flex-direction: column;\n}\n\#custom_select {\n    margin-bottom: 5px;\n}\n\#custom_select_list\.select-hidden {\n    display: none;\n}\n\#custom_select_list {\n    display: inline-flex;\n    flex-direction: column;\n    padding-left: 0;\n}\n\.custom-select__option:not\(:last-child\) {\n    margin-bottom: 5x;\n}\n<\/style>\n<section class=\"select-wrapper\">\n    <div class=\"btn btn-primary\" id=\"custom_select\">\n    <span class=\"select-active-text\"><\/span>\n    <span class=\"glyphicon glyphicon-chevron-down\"><\/span>\n    <\/div>\n    <ul class=\"select-hidden\" id=\"custom_select_list\">\n    <li>\n    <a class=\"custom-select__option\"\nhref=\"javascript:window\.pxcDocRedirect\(\)\">With Percona XtraDB Cluster<\/a>\n<\/li>\n<li>\n<a class=\"custom-select__option\"  href=\"javascript:window\.psDocRedirect\(\)\">With Percona Server for MySQL<\/a>\n<\/li>\n<\/ul>\n<\/section>\n<script>\nwindow\.psDocRedirect = \(\) => {\n    let newLink = window\.location\.href\.replace\(\"\/pxc\/\", \"\/ps\/\"\)\.replace\(\"for-pxc\/\", \"for-mysql\/ps\/\"\);\n    console\.log\(newLink\);\n    window\.location\.href = newLink;\n};\nwindow\.pxcDocRedirect = \(\) => {\n    let newLink = window\.location\.href\.replace\(\"\/ps\/\", \"\/pxc\/\"\);\n    console\.log\(newLink\);\n    window\.location\.href = newLink;\n};\n<\/script>\n<script>\nsetTimeout\(\(\) => {\n    makeSelect\(\);\n}, 500\);\nfunction makeSelect\(\) {\n    const custom_select = document\.getElementById\(\"custom_select\"\);\n    const select_active_option = custom_select\.getElementsByClassName\(\"select-active-text\"\)\[0\];\n    const custom_select_list = document\.getElementById\(\"custom_select_list\"\);\n    select_active_option\.innerHTML = window\.location\.href\.includes\(\"ps\"\) ?\n        custom_select_list\.getElementsByClassName\(\"custom-select__option\"\)\[1\]\.innerHTML :\n        custom_select_list\.getElementsByClassName\(\"custom-select__option\"\)\[0\]\.innerHTML;\n    document\.addEventListener\(\"click\", event => {\n        if \(event\.target\.parentElement\.id === \"custom_select\" || event\.target\.id === \"custom_select\"\) {\n            custom_select_list\.classList\.toggle\(\"select-hidden\"\)\n        }\n        if \(Array\.from\(event\.target\.classList\)\.includes\(\"custom-select__option\"\)\) {\n            select_active_option\.innerHTML = event\.target\.innerHTML;\n        }\n        if \(event\.target\.id !== \"custom_select\"\) { if \(event\.target\.parentElement\.id !== \"custom_select\"\) { custom_select_list\.classList\.add\(\"select-hidden\"\) } }\n    }\);\n}\n<\/script>/' source/percona-theme/localtoc.html
	@echo "Building html doc"

	$(SPHINXBUILD) -b html $(ALLSPHINXOPTS) $(BUILDDIR)/html
	@echo
	@echo "Build finished. The HTML pages are in $(BUILDDIR)/html."

netlify:
	$(SPHINXBUILD) -b html $(ALLSPHINXOPTS) -c source/conf-netlify $(BUILDDIR)/html
	@echo
	@echo "Netlify build finished. The HTML pages are in $(BUILDDIR)/html."

dirhtml:
	$(SPHINXBUILD) -b dirhtml $(ALLSPHINXOPTS) $(BUILDDIR)/dirhtml
	@echo
	@echo "Build finished. The HTML pages are in $(BUILDDIR)/dirhtml."

offhtml:
	$(SPHINXBUILD) -b html $(ALLSPHINXOPTS) $(BUILDDIR)/html
	@echo
	@echo "Build finished. The HTML pages are in $(BUILDDIR)/html."

singlehtml:
	$(SPHINXBUILD) -b singlehtml $(ALLSPHINXOPTS) $(BUILDDIR)/singlehtml
	@echo
	@echo "Build finished. The HTML page is in $(BUILDDIR)/singlehtml."

pickle:
	$(SPHINXBUILD) -b pickle $(ALLSPHINXOPTS) $(BUILDDIR)/pickle
	@echo
	@echo "Build finished; now you can process the pickle files."

json:
	$(SPHINXBUILD) -b json $(ALLSPHINXOPTS) $(BUILDDIR)/json
	@echo
	@echo "Build finished; now you can process the JSON files."

htmlhelp:
	$(SPHINXBUILD) -b htmlhelp $(ALLSPHINXOPTS) $(BUILDDIR)/htmlhelp
	@echo
	@echo "Build finished; now you can run HTML Help Workshop with the" \
	      ".hhp project file in $(BUILDDIR)/htmlhelp."

qthelp:
	$(SPHINXBUILD) -b qthelp $(ALLSPHINXOPTS) $(BUILDDIR)/qthelp
	@echo
	@echo "Build finished; now you can run "qcollectiongenerator" with the" \
	      ".qhcp project file in $(BUILDDIR)/qthelp, like this:"
	@echo "# qcollectiongenerator $(BUILDDIR)/qthelp/PerconaServer.qhcp"
	@echo "To view the help file:"
	@echo "# assistant -collectionFile $(BUILDDIR)/qthelp/PerconaServer.qhc"

devhelp:
	$(SPHINXBUILD) -b devhelp $(ALLSPHINXOPTS) $(BUILDDIR)/devhelp
	@echo
	@echo "Build finished."
	@echo "To view the help file:"
	@echo "# mkdir -p $$HOME/.local/share/devhelp/PerconaServer"
	@echo "# ln -s $(BUILDDIR)/devhelp $$HOME/.local/share/devhelp/PerconaServer"
	@echo "# devhelp"

epub:
	$(SPHINXBUILD) -b epub $(ALLSPHINXOPTS) $(BUILDDIR)/epub
	@echo
	@echo "Build finished. The epub file is in $(BUILDDIR)/epub."

latex:
	$(SPHINXBUILD) -b latex $(ALLSPHINXOPTS) $(BUILDDIR)/latex
	@echo
	@echo "Build finished; the LaTeX files are in $(BUILDDIR)/latex."
	@echo "Run \`make' in that directory to run these through (pdf)latex" \
	      "(use \`make latexpdf' here to do that automatically)."

latexpdf:
	@for i in ./source/*.rst; do sed -i '/\.\. figure::/s/\.svg/\.pdf/g' "$$i"; done
	@for i in ./source/*.rst; do sed -i '/\.\. image::/s/\.svg/\.pdf/g' "$$i"; done
	@for i in source/*.rst; do sed  '/\.\. figure::/s/\.svg/\.pdf/g' "$i"; done
	$(SPHINXBUILD) -b latex $(ALLSPHINXOPTS) $(BUILDDIR)/latex
	@echo "Running LaTeX files through pdflatex..."
	make -C $(BUILDDIR)/latex all-pdf
	@for i in ./source/*.rst; do sed -i '/\.\. figure::/s/\.pdf/\.svg/g' "$$i"; done
	@for i in ./source/*.rst; do sed -i '/\.\. image::/s/\.pdf/\.svg/g' "$$i"; done
	@echo "pdflatex finished; the PDF files are in $(BUILDDIR)/latex."

text:
	$(SPHINXBUILD) -b text $(ALLSPHINXOPTS) $(BUILDDIR)/text
	@echo
	@echo "Build finished. The text files are in $(BUILDDIR)/text."

man:
	$(SPHINXBUILD) -b man $(ALLSPHINXOPTS) $(BUILDDIR)/man
	@echo
	@echo "Build finished. The manual pages are in $(BUILDDIR)/man."

changes:
	$(SPHINXBUILD) -b changes $(ALLSPHINXOPTS) $(BUILDDIR)/changes
	@echo
	@echo "The overview file is in $(BUILDDIR)/changes."

linkcheck:
	$(SPHINXBUILD) -b linkcheck $(ALLSPHINXOPTS) $(BUILDDIR)/linkcheck
	@echo
	@echo "Link check complete; look for any errors in the above output " \
	      "or in $(BUILDDIR)/linkcheck/output.txt."

doctest:
	$(SPHINXBUILD) -b doctest $(ALLSPHINXOPTS) $(BUILDDIR)/doctest
	@echo "Testing of doctests in the sources finished, look at the " \
	      "results in $(BUILDDIR)/doctest/output.txt."
