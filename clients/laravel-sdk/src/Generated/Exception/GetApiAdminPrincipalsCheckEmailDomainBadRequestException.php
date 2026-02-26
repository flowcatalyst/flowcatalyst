<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalsCheckEmailDomainBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse400
     */
    private $apiAdminPrincipalsCheckEmailDomainGetResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse400 $apiAdminPrincipalsCheckEmailDomainGetResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsCheckEmailDomainGetResponse400 = $apiAdminPrincipalsCheckEmailDomainGetResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsCheckEmailDomainGetResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse400
    {
        return $this->apiAdminPrincipalsCheckEmailDomainGetResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}