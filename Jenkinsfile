region='us-central1-a'
testUrlPrefix="https://percona-jenkins-artifactory-public.s3.amazonaws.com/cloud-pxc-operator"
tests=[]

void createCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            NODES_NUM=3
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            ret_num=0
            while [ \${ret_num} -lt 15 ]; do
                ret_val=0
                gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
                gcloud config set project $GCP_PROJECT
                gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_SUFFIX} --zone $region --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $region --quiet || true

                gcloud container clusters create --zone $region $CLUSTER_NAME-${CLUSTER_SUFFIX} --cluster-version=1.28 --machine-type=n1-standard-4 --preemptible --disk-size 30 --num-nodes=\$NODES_NUM --network=jenkins-vpc --subnetwork=jenkins-${CLUSTER_SUFFIX} --no-enable-autoupgrade --cluster-ipv4-cidr=/21 --labels delete-cluster-after-hours=6 && \
                kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user jenkins@"$GCP_PROJECT".iam.gserviceaccount.com || ret_val=\$?
                if [ \${ret_val} -eq 0 ]; then break; fi
                ret_num=\$((ret_num + 1))
            done
            if [ \${ret_num} -eq 15 ]; then
                gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_SUFFIX} --zone $region --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $region --quiet || true
                exit 1
            fi
        """
   }
}

void shutdownCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
            gcloud config set project $GCP_PROJECT
            for namespace in \$(kubectl get namespaces --no-headers | awk '{print \$1}' | grep -vE "^kube-|^openshift" | sed '/-operator/ s/^/1-/' | sort | sed 's/^1-//'); do
                kubectl delete deployments --all -n \$namespace --force --grace-period=0 || true
                kubectl delete sts --all -n \$namespace --force --grace-period=0 || true
                kubectl delete replicasets --all -n \$namespace --force --grace-period=0 || true
                kubectl delete poddisruptionbudget --all -n \$namespace --force --grace-period=0 || true
                kubectl delete services --all -n \$namespace --force --grace-period=0 || true
                kubectl delete pods --all -n \$namespace --force --grace-period=0 || true
            done
            kubectl get svc --all-namespaces || true
            gcloud container clusters delete --zone $region $CLUSTER_NAME-${CLUSTER_SUFFIX}
        """
   }
}

void deleteOldClusters(String FILTER) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            if gcloud --version > /dev/null 2>&1; then
                gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
                gcloud config set project $GCP_PROJECT
                for GKE_CLUSTER in \$(gcloud container clusters list --format='csv[no-heading](name)' --filter="$FILTER"); do
                    GKE_CLUSTER_STATUS=\$(gcloud container clusters list --format='csv[no-heading](status)' --filter="\$GKE_CLUSTER")
                    retry=0
                    while [ "\$GKE_CLUSTER_STATUS" == "PROVISIONING" ]; do
                        echo "Cluster \$GKE_CLUSTER is being provisioned, waiting before delete."
                        sleep 10
                        GKE_CLUSTER_STATUS=\$(gcloud container clusters list --format='csv[no-heading](status)' --filter="\$GKE_CLUSTER")
                        let retry+=1
                        if [ \$retry -ge 60 ]; then
                            echo "Cluster \$GKE_CLUSTER to delete is being provisioned for too long. Skipping..."
                            break
                        fi
                    done
                    gcloud container clusters delete --async --zone $region --quiet \$GKE_CLUSTER || true
                done
            fi
        """
   }
}

void pushLogFile(String FILE_NAME) {
    def LOG_FILE_PATH="e2e-tests/logs/${FILE_NAME}.log"
    def LOG_FILE_NAME="${FILE_NAME}.log"
    echo "Push logfile $LOG_FILE_NAME file to S3!"
    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            S3_PATH=s3://percona-jenkins-artifactory-public/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 ls \$S3_PATH/${LOG_FILE_NAME} || :
            aws s3 cp --content-type text/plain --quiet ${LOG_FILE_PATH} \$S3_PATH/${LOG_FILE_NAME} || :
        """
    }
}

void pushArtifactFile(String FILE_NAME) {
    echo "Push $FILE_NAME file to S3!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            touch ${FILE_NAME}
            S3_PATH=s3://percona-jenkins-artifactory/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 ls \$S3_PATH/${FILE_NAME} || :
            aws s3 cp --quiet ${FILE_NAME} \$S3_PATH/${FILE_NAME} || :
        """
    }
}

void initTests() {
    echo "Populating tests into the tests array!"

    def records = readCSV file: 'e2e-tests/run-pr.csv'

    for (int i=0; i<records.size(); i++) {
        tests.add(["name": records[i][0], "mysql_ver": records[i][1], "cluster": "NA", "result": "skipped", "time": "0"])
    }

    markPassedTests()
}

void markPassedTests() {
    echo "Marking passed tests in the tests map!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            aws s3 ls "s3://percona-jenkins-artifactory/${JOB_NAME}/${env.GIT_SHORT_COMMIT}/" || :
        """

        for (int i=0; i<tests.size(); i++) {
            def testNameWithMysqlVersion = tests[i]["name"] +"-"+ tests[i]["mysql_ver"].replace(".", "-")
            def file="${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion"
            def retFileExists = sh(script: "aws s3api head-object --bucket percona-jenkins-artifactory --key ${JOB_NAME}/${env.GIT_SHORT_COMMIT}/${file} >/dev/null 2>&1", returnStatus: true)

            if (retFileExists == 0) {
                tests[i]["result"] = "passed"
            }
        }
    }
}

void printKubernetesStatus(String LOCATION, String CLUSTER_SUFFIX) {
    sh """
        export KUBECONFIG=/tmp/$CLUSTER_NAME-$CLUSTER_SUFFIX
        echo "========== KUBERNETES STATUS $LOCATION TEST =========="
        gcloud container clusters list|grep -E "NAME|$CLUSTER_NAME-$CLUSTER_SUFFIX "
        echo
        kubectl get nodes
        echo
        kubectl top nodes
        echo
        kubectl get pods --all-namespaces
        echo
        kubectl top pod --all-namespaces
        echo
        kubectl get events --field-selector type!=Normal --all-namespaces
        echo "======================================================"
    """
}

TestsReport = '| Test name  | Status |\r\n| ------------- | ------------- |'
TestsReportXML = '<testsuite name=\\"PXC\\">\n'

void makeReport() {
    def wholeTestAmount=tests.size()
    def startedTestAmount = 0

    for (int i=0; i<tests.size(); i++) {
        def testResult = tests[i]["result"]
        def testTime = tests[i]["time"]
        def testNameWithMysqlVersion = tests[i]["name"] +"-"+ tests[i]["mysql_ver"].replace(".", "-")
        def testUrl = "${testUrlPrefix}/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${testNameWithMysqlVersion}.log"

        if (tests[i]["result"] != "skipped") {
            startedTestAmount++
        }
        TestsReport = TestsReport + "\r\n| "+ testNameWithMysqlVersion +" | ["+ tests[i]["result"] +"]("+ testUrl +") |"
        TestsReportXML = TestsReportXML + '<testcase name=\\"' + testNameWithMysqlVersion + '\\" time=\\"' + testTime + '\\"><'+ testResult +'/></testcase>\n'
    }
    TestsReport = TestsReport + "\r\n| We run $startedTestAmount out of $wholeTestAmount|"
    TestsReportXML = TestsReportXML + '</testsuite>\n'
}

void clusterRunner(String cluster) {
    def clusterCreated=0

    for (int i=0; i<tests.size(); i++) {
        if (tests[i]["result"] == "skipped" && currentBuild.nextBuild == null) {
            tests[i]["result"] = "failure"
            tests[i]["cluster"] = cluster
            if (clusterCreated == 0) {
                createCluster(cluster)
                clusterCreated++
            }
            runTest(i)
        }
    }

    if (clusterCreated >= 1) {
        shutdownCluster(cluster)
    }
}

void runTest(Integer TEST_ID) {
    def retryCount = 0
    def testName = tests[TEST_ID]["name"]
    def mysqlVer = tests[TEST_ID]["mysql_ver"]
    def clusterSuffix = tests[TEST_ID]["cluster"]
    def testNameWithMysqlVersion = "$testName-$mysqlVer".replace(".", "-")

    waitUntil {
        def timeStart = new Date().getTime()
        try {
            echo "The $testName-$mysqlVer test was started on cluster $CLUSTER_NAME-$clusterSuffix !"
            tests[TEST_ID]["result"] = "failure"

            timeout(time: 90, unit: 'MINUTES') {
                sh """
                    if [ $retryCount -eq 0 ]; then
                        export DEBUG_TESTS=0
                    else
                        export DEBUG_TESTS=1
                    fi
                    export KUBECONFIG=/tmp/$CLUSTER_NAME-$clusterSuffix
                    export MYSQL_VERSION=$mysqlVer
                    time bash e2e-tests/$testName/run
                """
            }
            pushArtifactFile("${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion")
            tests[TEST_ID]["result"] = "passed"
            return true
        }
        catch (exc) {
            printKubernetesStatus("AFTER","$clusterSuffix")
            if (retryCount >= 1 || currentBuild.nextBuild != null) {
                currentBuild.result = 'FAILURE'
                return true
            }
            retryCount++
            return false
        }
        finally {
            def timeStop = new Date().getTime()
            def durationSec = (timeStop - timeStart) / 1000
            tests[TEST_ID]["time"] = durationSec
            pushLogFile("$testNameWithMysqlVersion")
            echo "The $testName-$mysqlVer test was finished!"
        }
    }
}

def skipBranchBuilds = true
if (env.CHANGE_URL) {
    skipBranchBuilds = false
}

pipeline {
    environment {
        CLOUDSDK_CORE_DISABLE_PROMPTS = 1
        CLEAN_NAMESPACE = 1
        OPERATOR_NS = 'pxc-operator'
        GIT_SHORT_COMMIT = sh(script: 'git rev-parse --short HEAD', , returnStdout: true).trim()
        VERSION = "${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}"
        CLUSTER_NAME = sh(script: "echo jen-pxc-${env.CHANGE_ID}-${GIT_SHORT_COMMIT}-${env.BUILD_NUMBER} | tr '[:upper:]' '[:lower:]'", , returnStdout: true).trim()
        AUTHOR_NAME = sh(script: "echo ${CHANGE_AUTHOR_EMAIL} | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
        ENABLE_LOGGING = "true"
    }
    agent {
        label 'docker'
    }
    options {
        disableConcurrentBuilds(abortPrevious: true)
    }
    stages {
        stage('Prepare') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                initTests()
                script {
                    if (AUTHOR_NAME == 'null') {
                        AUTHOR_NAME = sh(script: "git show -s --pretty=%ae | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
                    }
                    for (comment in pullRequest.comments) {
                        println("Author: ${comment.user}, Comment: ${comment.body}")
                        if (comment.user.equals('JNKPercona')) {
                            println("delete comment")
                            comment.delete()
                        }
                    }
                }
                sh """
                    sudo curl -s -L -o /usr/local/bin/kubectl https://dl.k8s.io/release/\$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl && sudo chmod +x /usr/local/bin/kubectl
                    kubectl version --client --output=yaml

                    curl -fsSL https://get.helm.sh/helm-v3.12.3-linux-amd64.tar.gz | sudo tar -C /usr/local/bin --strip-components 1 -xzf - linux-amd64/helm

                    sudo curl -fsSL https://github.com/mikefarah/yq/releases/download/v4.44.1/yq_linux_amd64 -o /usr/local/bin/yq && sudo chmod +x /usr/local/bin/yq
                    sudo curl -fsSL https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux64 -o /usr/local/bin/jq && sudo chmod +x /usr/local/bin/jq

                    sudo tee /etc/yum.repos.d/google-cloud-sdk.repo << EOF
[google-cloud-cli]
name=Google Cloud CLI
baseurl=https://packages.cloud.google.com/yum/repos/cloud-sdk-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=0
gpgkey=https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF
                    sudo yum install -y google-cloud-cli google-cloud-cli-gke-gcloud-auth-plugin

                    curl -sL https://github.com/mitchellh/golicense/releases/latest/download/golicense_0.2.0_linux_x86_64.tar.gz | sudo tar -C /usr/local/bin -xzf - golicense

                    sudo yum install -y https://repo.percona.com/yum/percona-release-latest.noarch.rpm || true
                    sudo percona-release enable-only tools
                    sudo yum install -y percona-xtrabackup-80 | true
                """

                withCredentials([file(credentialsId: 'cloud-secret-file', variable: 'CLOUD_SECRET_FILE')]) {
                    sh '''
                        cp $CLOUD_SECRET_FILE e2e-tests/conf/cloud-secret.yml
                    '''
                }
                deleteOldClusters("jen-pxc-$CHANGE_ID")
            }
        }
        stage('Build docker image') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                withCredentials([usernamePassword(credentialsId: 'hub.docker.com', passwordVariable: 'PASS', usernameVariable: 'USER')]) {
                    sh '''
                        DOCKER_TAG=perconalab/percona-xtradb-cluster-operator:$VERSION
                        docker_tag_file='./results/docker/TAG'
                        mkdir -p $(dirname ${docker_tag_file})
                        echo ${DOCKER_TAG} > "${docker_tag_file}"
                            sg docker -c "
                                docker login -u '${USER}' -p '${PASS}'
                                export RELEASE=0
                                export IMAGE=\$DOCKER_TAG
                                ./e2e-tests/build
                                docker logout
                            "
                        sudo rm -rf ./build
                    '''
                }
                stash includes: 'results/docker/TAG', name: 'IMAGE'
                archiveArtifacts 'results/docker/TAG'
            }
        }
        stage('GoLicenseDetector test') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                sh """
                    mkdir -p $WORKSPACE/src/github.com/percona
                    ln -s $WORKSPACE $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator
                    sg docker -c "
                        docker run \
                            --rm \
                            -v $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator:/go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -w /go/src/github.com/percona/percona-xtradb-cluster-operator \
                            golang:1.22 sh -c '
                                go install -mod=readonly github.com/google/go-licenses@latest;
                                /go/bin/go-licenses csv github.com/percona/percona-xtradb-cluster-operator/cmd/manager \
                                    | cut -d , -f 3 \
                                    | sort -u \
                                    > go-licenses-new || :
                            '
                    "
                    diff -u e2e-tests/license/compare/go-licenses go-licenses-new
                """
            }
        }
        stage('GoLicense test') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                sh '''
                    mkdir -p $WORKSPACE/src/github.com/percona
                    ln -s $WORKSPACE $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator
                    sg docker -c "
                        docker run \
                            --rm \
                            -v $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator:/go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -w /go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -e GO111MODULE=on \
                            -e GOFLAGS='-buildvcs=false' \
                            golang:1.22 sh -c 'go build -v -o percona-xtradb-cluster-operator github.com/percona/percona-xtradb-cluster-operator/cmd/manager'
                    "
                '''

                withCredentials([string(credentialsId: 'GITHUB_API_TOKEN', variable: 'GITHUB_TOKEN')]) {
                    sh """
                        golicense -plain ./percona-xtradb-cluster-operator \
                            | grep -v 'license not found' \
                            | sed -r 's/^[^ ]+[ ]+//' \
                            | sort \
                            | uniq \
                            > golicense-new || true
                        diff -u e2e-tests/license/compare/golicense golicense-new
                    """
                }
            }
        }
        stage('Run tests for operator') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            options {
                timeout(time: 4, unit: 'HOURS')
            }
            parallel {
                stage('cluster1') {
                    steps {
                        clusterRunner('cluster1')
                    }
                }
                stage('cluster2') {
                    steps {
                        clusterRunner('cluster2')
                    }
                }
                stage('cluster3') {
                    steps {
                        clusterRunner('cluster3')
                    }
                }
                stage('cluster4') {
                    steps {
                        clusterRunner('cluster4')
                    }
                }
                stage('cluster5') {
                    steps {
                        clusterRunner('cluster5')
                    }
                }
                stage('cluster6') {
                    steps {
                        clusterRunner('cluster6')
                    }
                }
                stage('cluster7') {
                    steps {
                        clusterRunner('cluster7')
                    }
                }
                stage('cluster8') {
                    steps {
                        clusterRunner('cluster8')
                    }
                }
                stage('cluster9') {
                    steps {
                        clusterRunner('cluster9')
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                echo "CLUSTER ASSIGNMENTS\n" + tests.toString().replace("], ","]\n").replace("]]","]").replaceFirst("\\[","")

                if (currentBuild.result != null && currentBuild.result != 'SUCCESS' && currentBuild.nextBuild == null) {
                    try {
                        slackSend channel: "@${AUTHOR_NAME}", color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                    catch (exc) {
                        slackSend channel: '#cloud-dev-ci', color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                }

                if (env.CHANGE_URL && currentBuild.nextBuild == null) {
                    for (comment in pullRequest.comments) {
                        println("Author: ${comment.user}, Comment: ${comment.body}")
                        if (comment.user.equals('JNKPercona')) {
                            println("delete comment")
                            comment.delete()
                        }
                    }
                    makeReport()
                    sh """
                        echo "${TestsReportXML}" > TestsReport.xml
                    """
                    step([$class: 'JUnitResultArchiver', testResults: '*.xml', healthScaleFactor: 1.0])
                    archiveArtifacts '*.xml'

                    unstash 'IMAGE'
                    def IMAGE = sh(returnStdout: true, script: "cat results/docker/TAG").trim()
                    TestsReport = TestsReport + "\r\n\r\ncommit: ${env.CHANGE_URL}/commits/${env.GIT_COMMIT}\r\nimage: `${IMAGE}`\r\n"
                    pullRequest.comment(TestsReport)
                }
            }
            deleteOldClusters("$CLUSTER_NAME")
            sh """
                sudo docker system prune --volumes -af
                sudo rm -rf *
            """
            deleteDir()
        }
    }
}
