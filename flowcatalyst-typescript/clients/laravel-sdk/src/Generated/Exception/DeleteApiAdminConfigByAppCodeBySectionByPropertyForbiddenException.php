<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminConfigByAppCodeBySectionByPropertyForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403
     */
    private $apiAdminConfigAppCodeSectionPropertyDeleteResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403 $apiAdminConfigAppCodeSectionPropertyDeleteResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyDeleteResponse403 = $apiAdminConfigAppCodeSectionPropertyDeleteResponse403;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyDeleteResponse403(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403
    {
        return $this->apiAdminConfigAppCodeSectionPropertyDeleteResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}