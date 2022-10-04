GKERegion='us-central1-a'

void CreateCluster(String CLUSTER_PREFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            NODES_NUM=3
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_PREFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
            gcloud config set project $GCP_PROJECT
            gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_PREFIX} --zone $GKERegion --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
            gcloud container clusters create --zone $GKERegion $CLUSTER_NAME-${CLUSTER_PREFIX} --cluster-version=1.21 --machine-type=n1-standard-4 --preemptible --num-nodes=\$NODES_NUM --network=jenkins-vpc --subnetwork=jenkins-${CLUSTER_PREFIX} --no-enable-autoupgrade
            kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user jenkins@"$GCP_PROJECT".iam.gserviceaccount.com
        """
   }
}
void ShutdownCluster(String CLUSTER_PREFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_PREFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
            gcloud config set project $GCP_PROJECT
            gcloud container clusters delete --zone $GKERegion $CLUSTER_NAME-${CLUSTER_PREFIX}
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

void popArtifactFile(String FILE_NAME) {
    echo "Try to get $FILE_NAME file from S3!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            S3_PATH=s3://percona-jenkins-artifactory/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 cp --quiet \$S3_PATH/${FILE_NAME} ${FILE_NAME} || :
        """
    }
}

TestsReport = '| Test name  | Status |\r\n| ------------- | ------------- |'
testsReportMap  = [:]
testsResultsMap = [:]

void makeReport() {
    def wholeTestAmount=sh(script: 'cat e2e-tests/run | grep "|| fail"| wc -l', , returnStdout: true).trim()
    def startedTestAmount = testsReportMap.size()
    for ( test in testsReportMap ) {
        TestsReport = TestsReport + "\r\n| ${test.key} | ${test.value} |"
    }
    TestsReport = TestsReport + "\r\n| We run $startedTestAmount out of $wholeTestAmount|"
}

void setTestsresults() {
    testsResultsMap.each { file ->
        pushArtifactFile("${file.key}")
    }
}

void runTest(String TEST_NAME, String CLUSTER_PREFIX, String MYSQL_VERSION) {
    def retryCount = 0
    def testNameWithMysqlVersion = "$TEST_NAME-$MYSQL_VERSION".replace(".", "-")
    waitUntil {
        def testUrl = "https://percona-jenkins-artifactory-public.s3.amazonaws.com/cloud-pxc-operator/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${testNameWithMysqlVersion}.log"
        echo " test url is $testUrl"
        try {
            echo "The $TEST_NAME test was started!"
            testsReportMap["$testNameWithMysqlVersion"] = "[failed]($testUrl)"
            popArtifactFile("${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion")

            timeout(time: 90, unit: 'MINUTES') {
                sh """
                    if [ -f "${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion" ]; then
                        echo Skip $TEST_NAME test
                    else
                        export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_PREFIX}
                        export MYSQL_VERSION=$MYSQL_VERSION
                        source $HOME/google-cloud-sdk/path.bash.inc
                        time bash ./e2e-tests/$TEST_NAME/run
                    fi
                """
            }
            echo "end test url is $testUrl"
            testsReportMap["$testNameWithMysqlVersion"] = "[passed]($testUrl)"
            testsResultsMap["${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion"] = 'passed'
            return true
        }
        catch (exc) {
            if (retryCount >= 2) {
                currentBuild.result = 'FAILURE'
                return true
            }
            retryCount++
            return false
        }
        finally {
            pushLogFile("$testNameWithMysqlVersion")
            echo "The $TEST_NAME test was finished!"
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
        GIT_SHORT_COMMIT = sh(script: 'git describe --always --dirty', , returnStdout: true).trim()
        VERSION = "${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}"
        CLUSTER_NAME = sh(script: "echo jenkins-pxc-${GIT_SHORT_COMMIT} | tr '[:upper:]' '[:lower:]'", , returnStdout: true).trim()
        AUTHOR_NAME  = sh(script: "echo ${CHANGE_AUTHOR_EMAIL} | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
        ENABLE_LOGGING="true"
    }
    agent {
        label 'docker'
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
                stage('E2E Upgrade') {
                    steps {
                        CreateCluster('upgrade')
                        runTest('upgrade-haproxy', 'upgrade', '8.0')
                        ShutdownCluster('upgrade')
                        CreateCluster('upgrade')
                        runTest('upgrade-proxysql', 'upgrade', '8.0')
                        ShutdownCluster('upgrade')
                        CreateCluster('upgrade')
                        runTest('smart-update', 'upgrade', '8.0')
                        runTest('upgrade-consistency', 'upgrade', '8.0')
                        ShutdownCluster('upgrade')
                    }
                }
                stage('E2E Basic Tests') {
                    steps {
                        CreateCluster('basic')
                        runTest('haproxy', 'basic', '8.0')
                        runTest('init-deploy', 'basic', '8.0')
                        runTest('limits', 'basic', '8.0')
                        runTest('monitoring-2-0', 'basic', '8.0')
                        runTest('affinity', 'basic', '8.0')
                        runTest('one-pod', 'basic', '8.0')
                        runTest('auto-tuning', 'basic', '8.0')
                        runTest('proxysql-sidecar-res-limits', 'basic', '8.0')
                        runTest('users', 'basic', '8.0')
                        runTest('tls-issue-self','basic', '8.0')
                        runTest('tls-issue-cert-manager','basic', '8.0')
                        runTest('tls-issue-cert-manager-ref','basic', '8.0')
                        runTest('validation-hook','basic', '8.0')
                        runTest('proxy-protocol','basic', '8.0')
                        ShutdownCluster('basic')
                    }
                }
                stage('E2E Scaling') {
                    steps {
                        CreateCluster('scaling')
                        runTest('scaling', 'scaling', '8.0')
                        runTest('scaling-proxysql', 'scaling', '8.0')
                        runTest('security-context', 'scaling', '8.0')
                        ShutdownCluster('scaling')
                    }
                }
                stage('E2E SelfHealing') {
                    steps {
                        CreateCluster('selfhealing')
                        runTest('storage', 'selfhealing', '8.0')
                        runTest('self-healing', 'selfhealing', '8.0')
                        runTest('self-healing-chaos', 'selfhealing', '8.0')
                        runTest('self-healing-advanced', 'selfhealing', '8.0')
                        runTest('self-healing-advanced-chaos', 'selfhealing', '8.0')
                        runTest('operator-self-healing', 'selfhealing', '8.0')
                        runTest('operator-self-healing-chaos', 'selfhealing', '8.0')
                        ShutdownCluster('selfhealing')
                    }
                }
                stage('E2E Backups') {
                    steps {
                        CreateCluster('backups')
                        runTest('recreate', 'backups', '8.0')
                        runTest('restore-to-encrypted-cluster', 'backups', '8.0')
                        runTest('demand-backup', 'backups', '8.0')
                        runTest('pitr', 'backups', '8.0')
                        runTest('demand-backup-encrypted-with-tls', 'backups', '8.0')
                        ShutdownCluster('backups')
                    }
                }
                stage('E2E Scheduled-backups') {
                    steps {
                        CreateCluster('scheduled-backups')
                        runTest('scheduled-backup', 'scheduled-backups', '8.0')
                        ShutdownCluster('scheduled-backups')
                    }
                }
                stage('E2E BigData') {
                    steps {
                        CreateCluster('bigdata')
                        runTest('big-data', 'bigdata', '8.0')
                        ShutdownCluster('bigdata')
                    }
                }
                stage('E2E CrossSite') {
                    steps {
                        CreateCluster('cross-site')
                        runTest('cross-site', 'cross-site', '8.0')
                        ShutdownCluster('cross-site')
                    }
                }
                stage('E2E Mysql 5.7') {
                    steps {
                        CreateCluster('mysql-57')
                        runTest('users', 'mysql-57', '5.7')
                        runTest('one-pod', 'mysql-57', '5.7')
                        runTest('scheduled-backup', 'mysql-57', '5.7')
                        runTest('init-deploy', 'mysql-57', '5.7')
                        runTest('haproxy', 'mysql-57', '5.7')
                        ShutdownCluster('mysql-57')
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                setTestsresults()
                if (currentBuild.result != null && currentBuild.result != 'SUCCESS') {

                    try {
                        slackSend channel: "@${AUTHOR_NAME}", color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                    catch (exc) {
                        slackSend channel: '#cloud-dev-ci', color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                }
                if (env.CHANGE_URL) {
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
            withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
                sh """
                    if [ -f $HOME/google-cloud-sdk/path.bash.inc ]; then
                        source $HOME/google-cloud-sdk/path.bash.inc
                        gcloud auth activate-service-account --key-file \$CLIENT_SECRET_FILE
                        gcloud config set project \$GCP_PROJECT
                        gcloud container clusters list --format='csv[no-heading](name)' --filter $CLUSTER_NAME | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
                    fi
                    sudo docker system prune -fa
                    sudo rm -rf ./*
                    sudo rm -rf $HOME/google-cloud-sdk
                """
            }
            deleteDir()
        }
    }
}
