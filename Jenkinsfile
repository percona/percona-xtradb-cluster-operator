GKERegion='us-central1-a'
testUrlPrefix="https://percona-jenkins-artifactory-public.s3.amazonaws.com/cloud-pxc-operator"

tests = [
    1:[name: "affinity", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    2:[name: "auto-tuning", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    3:[name: "cross-site", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    4:[name: "demand-backup-cloud", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    5:[name: "demand-backup-encrypted-with-tls", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    6:[name: "demand-backup", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    7:[name: "haproxy", mysql_ver: "5.7", cluster: "NA", result:"NA"],
    8:[name: "haproxy", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    9:[name: "init-deploy", mysql_ver: "5.7", cluster: "NA", result:"NA"],
    10:[name: "init-deploy", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    11:[name: "limits", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    12:[name: "monitoring-2-0", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    13:[name: "one-pod", mysql_ver: "5.7", cluster: "NA", result:"NA"],
    14:[name: "one-pod", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    15:[name: "pitr", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    16:[name: "proxy-protocol", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    17:[name: "proxysql-sidecar-res-limits", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    18:[name: "recreate", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    19:[name: "restore-to-encrypted-cluster", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    20:[name: "scaling-proxysql", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    21:[name: "scaling", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    22:[name: "scheduled-backup", mysql_ver: "5.7", cluster: "NA", result:"NA"],
    23:[name: "scheduled-backup", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    24:[name: "security-context", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    25:[name: "smart-update1", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    26:[name: "smart-update2", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    27:[name: "storage", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    28:[name: "tls-issue-cert-manager-ref", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    29:[name: "tls-issue-cert-manager", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    30:[name: "tls-issue-self", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    31:[name: "upgrade-consistency", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    32:[name: "upgrade-haproxy", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    33:[name: "upgrade-proxysql", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    34:[name: "users", mysql_ver: "5.7", cluster: "NA", result:"NA"],
    35:[name: "users", mysql_ver: "8.0", cluster: "NA", result:"NA"],
    36:[name: "validation-hook", mysql_ver: "8.0", cluster: "NA", result:"NA"]
]

void CreateCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            NODES_NUM=3
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            ret_num=0
            while [ \${ret_num} -lt 15 ]; do
                ret_val=0
                gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
                gcloud config set project $GCP_PROJECT
                gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_SUFFIX} --zone $GKERegion --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
                gcloud container clusters create --zone $GKERegion $CLUSTER_NAME-${CLUSTER_SUFFIX} --cluster-version=1.21 --machine-type=n1-standard-4 --preemptible --num-nodes=\$NODES_NUM --network=jenkins-vpc --subnetwork=jenkins-${CLUSTER_SUFFIX} --no-enable-autoupgrade --cluster-ipv4-cidr=/21 --labels delete-cluster-after-hours=6 && \
                kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user jenkins@"$GCP_PROJECT".iam.gserviceaccount.com || ret_val=\$?
                if [ \${ret_val} -eq 0 ]; then break; fi
                ret_num=\$((ret_num + 1))
            done
            if [ \${ret_num} -eq 15 ]; then
                gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_SUFFIX} --zone $GKERegion --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
                exit 1
            fi
        """
   }
}

void ShutdownCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
            gcloud config set project $GCP_PROJECT
            gcloud container clusters delete --zone $GKERegion $CLUSTER_NAME-${CLUSTER_SUFFIX}
        """
   }
}

void DeleteOldClusters(String FILTER) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            if [ -f $HOME/google-cloud-sdk/path.bash.inc ]; then
                export USE_GKE_GCLOUD_AUTH_PLUGIN=True
                source $HOME/google-cloud-sdk/path.bash.inc
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
                    gcloud container clusters delete --async --zone $GKERegion --quiet \$GKE_CLUSTER || true
                done
            fi
        """
   }
}

void pushLogFile(String FILE_NAME) {
    LOG_FILE_PATH="e2e-tests/logs/${FILE_NAME}.log"
    LOG_FILE_NAME="${FILE_NAME}.log"
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

void populatePassedTests() {
    echo "Populating passed tests into the tests map!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            aws s3 ls "s3://percona-jenkins-artifactory/${JOB_NAME}/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/" || :
        """

        for (int id=1; id<=tests.size(); id++) {
            def testNameWithMysqlVersion = tests[id]["name"] +"-"+ tests[id]["mysql_ver"].replace(".", "-")
            def file="${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion"
            def retFileExists = sh(script: "aws s3api head-object --bucket percona-jenkins-artifactory --key ${JOB_NAME}/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${file} >/dev/null 2>&1", returnStatus: true)

            if (retFileExists == 0) {
                tests[id]["result"] = "passed"
            }
        }
    }
}

void printKubernetesStatus(String LOCATION, String CLUSTER_SUFFIX) {
    sh """
        export KUBECONFIG=/tmp/$CLUSTER_NAME-$CLUSTER_SUFFIX
        export USE_GKE_GCLOUD_AUTH_PLUGIN=True
        source $HOME/google-cloud-sdk/path.bash.inc
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

void makeReport() {
    def wholeTestAmount=tests.size()
    def startedTestAmount = 0
    
    for (int id=1; id<=tests.size(); id++) {
        def testNameWithMysqlVersion = tests[id]["name"] +"-"+ tests[id]["mysql_ver"].replace(".", "-")
        def testUrl = "${testUrlPrefix}/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${testNameWithMysqlVersion}.log"

        if (tests[id]["result"] != "NA") {
            startedTestAmount++
        }
        TestsReport = TestsReport + "\r\n| "+ testNameWithMysqlVersion +" | ["+ tests[id]["result"] +"]("+ testUrl +") |"
    }
    TestsReport = TestsReport + "\r\n| We run $startedTestAmount out of $wholeTestAmount|"
}

void setTestsResults() {
    for (int id=1; id<=tests.size(); id++) {
        def testNameWithMysqlVersion = tests[id]["name"] +"-"+ tests[id]["mysql_ver"].replace(".", "-")
        def file="${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion"

        if (tests[id]["result"] == "passed") {
            pushArtifactFile(file)
        }
    }
}

void clusterRunner(String cluster) {
    for (int id=1; id<=tests.size(); id++) {
        if (tests[id]["result"] == "NA") {
            tests[id]["result"] = "failed"
            tests[id]["cluster"] = cluster
            runTest(id, tests[id]["cluster"])
        }
    }
}

void runTest(Integer TEST_ID, String CLUSTER_SUFFIX) {
    def retryCount = 0
    def testName = tests[TEST_ID]["name"]
    def mysqlVer = tests[TEST_ID]["mysql_ver"]
    def testNameWithMysqlVersion = "$testName-$mysqlVer".replace(".", "-")

    waitUntil {
        def testUrl = "${testUrlPrefix}/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${testNameWithMysqlVersion}.log"
        echo " test url is $testUrl"
        try {
            echo "The $testName test was started!"
            tests[TEST_ID]["result"] = "failed"

            timeout(time: 90, unit: 'MINUTES') {
                sh """
                    export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
                    export MYSQL_VERSION=$mysqlVer
                    source $HOME/google-cloud-sdk/path.bash.inc
                    time bash ./e2e-tests/$testName/run
                """
            }
            echo "end test url is $testUrl"
            tests[TEST_ID]["result"] = "passed"
            return true
        }
        catch (exc) {
            printKubernetesStatus("AFTER","$CLUSTER_SUFFIX")
            if (retryCount >= 1) {
                currentBuild.result = 'FAILURE'
                return true
            }
            retryCount++
            return false
        }
        finally {
            pushLogFile("$testNameWithMysqlVersion")
            echo "The $testName test was finished!"
        }
    }
}

void installRpms() {
    sh '''
        sudo yum install -y https://repo.percona.com/yum/percona-release-latest.noarch.rpm || true
        sudo percona-release enable-only tools
        sudo yum install -y percona-xtrabackup-80 jq | true
    '''
}

def skipBranchBuilds = true
if ( env.CHANGE_URL ) {
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
        AUTHOR_NAME  = sh(script: "echo ${CHANGE_AUTHOR_EMAIL} | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
        ENABLE_LOGGING="true"
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
                installRpms()
                populatePassedTests()
                script {
                    if ( AUTHOR_NAME == 'null' )  {
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
                sh '''
                    if [ ! -d $HOME/google-cloud-sdk/bin ]; then
                        rm -rf $HOME/google-cloud-sdk
                        curl https://sdk.cloud.google.com | bash
                    fi

                    source $HOME/google-cloud-sdk/path.bash.inc
                    gcloud components install alpha
                    gcloud components install kubectl
                    curl -fsSL https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
                    curl -s -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz \
                        | sudo tar -C /usr/local/bin --strip-components 1 --wildcards -zxvpf - '*/oc'

                    curl -s -L https://github.com/mitchellh/golicense/releases/latest/download/golicense_0.2.0_linux_x86_64.tar.gz \
                        | sudo tar -C /usr/local/bin --wildcards -zxvpf -

                    sudo sh -c "curl -s -L https://github.com/mikefarah/yq/releases/download/v4.27.2/yq_linux_amd64 > /usr/local/bin/yq"
                    sudo chmod +x /usr/local/bin/yq
                '''
                withCredentials([file(credentialsId: 'cloud-secret-file', variable: 'CLOUD_SECRET_FILE')]) {
                    sh '''
                        cp $CLOUD_SECRET_FILE ./e2e-tests/conf/cloud-secret.yml
                    '''
                }
                DeleteOldClusters("jen-pxc-$CHANGE_ID")
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
                            golang:1.19 sh -c '
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
                            golang:1.19 sh -c 'go build -v -o percona-xtradb-cluster-operator github.com/percona/percona-xtradb-cluster-operator/cmd/manager'
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
                timeout(time: 3, unit: 'HOURS')
            }
            parallel {
                stage('cluster1') {
                    steps {
                        CreateCluster('cluster1')
                        clusterRunner('cluster1')
                        ShutdownCluster('cluster1')
                    }
                }
                stage('cluster2') {
                    steps {
                        CreateCluster('cluster2')
                        clusterRunner('cluster2')
                        ShutdownCluster('cluster2')
                    }
                }
                stage('cluster3') {
                    steps {
                        CreateCluster('cluster3')
                        clusterRunner('cluster3')
                        ShutdownCluster('cluster3')
                    }
                }
                stage('cluster4') {
                    steps {
                        CreateCluster('cluster4')
                        clusterRunner('cluster4')
                        ShutdownCluster('cluster4')
                    }
                }
                stage('cluster5') {
                    steps {
                        CreateCluster('cluster5')
                        clusterRunner('cluster5')
                        ShutdownCluster('cluster5')
                    }
                }
                stage('cluster6') {
                    steps {
                        CreateCluster('cluster6')
                        clusterRunner('cluster6')
                        ShutdownCluster('cluster6')
                    }
                }
                stage('cluster7') {
                    steps {
                        CreateCluster('cluster7')
                        clusterRunner('cluster7')
                        ShutdownCluster('cluster7')
                    }
                }
                stage('cluster8') {
                    steps {
                        CreateCluster('cluster8')
                        clusterRunner('cluster8')
                        ShutdownCluster('cluster8')
                    }
                }
                stage('cluster9') {
                    steps {
                        CreateCluster('cluster9')
                        clusterRunner('cluster9')
                        ShutdownCluster('cluster9')
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                echo "CLUSTER ASSIGNMENTS\n" + tests.toString().replace("], ","]\n").replace("]]","]").replaceFirst("\\[","")
                setTestsResults()
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
                    unstash 'IMAGE'
                    def IMAGE = sh(returnStdout: true, script: "cat results/docker/TAG").trim()
                    TestsReport = TestsReport + "\r\n\r\ncommit: ${env.CHANGE_URL}/commits/${env.GIT_COMMIT}\r\nimage: `${IMAGE}`\r\n"
                    pullRequest.comment(TestsReport)
                }
            }
            DeleteOldClusters("$CLUSTER_NAME")
            sh """
                sudo docker system prune -fa
                sudo rm -rf ./*
                sudo rm -rf $HOME/google-cloud-sdk
            """
            deleteDir()
        }
    }
}
